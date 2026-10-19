package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func newEngine(log *slog.Logger) *gin.Engine {
	r := gin.New()
	r.Use(RequestID(), Logger(log), Recovery(log, func(c *gin.Context) {
		c.String(http.StatusInternalServerError, "custom 500")
	}))
	return r
}

func get(h http.Handler, path string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		reused   bool
	}{
		{"none", "", false},
		{"valid", "abc-123_x.y:z", true},
		{"uuid", "3f2b8c1e-9d4a-4f6b-8e2a-1c5d7e9f0a3b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fromCtx string
			r := gin.New()
			r.Use(RequestID())
			r.GET("/", func(c *gin.Context) { fromCtx = RequestIDFrom(c.Request.Context()) })

			header := http.Header{}
			if tt.incoming != "" {
				header.Set(HeaderRequestID, tt.incoming)
			}
			got := get(r, "/", header).Header().Get(HeaderRequestID)

			if got == "" {
				t.Fatal("no X-Request-ID in the response")
			}
			if fromCtx != got {
				t.Errorf("context ID %q, response header %q; want the same", fromCtx, got)
			}
			if reused := got == tt.incoming; reused != tt.reused {
				t.Errorf("ID %q reused = %v, want %v", got, reused, tt.reused)
			}
		})
	}
}

func TestRequestIDsAreUnique(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(*gin.Context) {})
	a := get(r, "/", nil).Header().Get(HeaderRequestID)
	b := get(r, "/", nil).Header().Get(HeaderRequestID)
	if a == b {
		t.Errorf("two requests got the same ID %q", a)
	}
}

func TestRequestIDFromEmptyContext(t *testing.T) {
	if id := RequestIDFrom(t.Context()); id != "" {
		t.Errorf("got %q, want empty", id)
	}
}

func TestLogger(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		status    int
		err       error
		wantLevel string
	}{
		{"ok", "/page", http.StatusOK, nil, "INFO"},
		{"client error", "/page", http.StatusBadRequest, errors.New("bad input"), "WARN"},
		{"server error", "/page", http.StatusBadGateway, errors.New("upstream down"), "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := newEngine(newLogger(&buf))
			r.GET(tt.path, func(c *gin.Context) {
				if tt.err != nil {
					_ = c.Error(tt.err)
				}
				c.String(tt.status, "hello")
			})
			w := get(r, tt.path+"?token=secret", http.Header{HeaderRequestID: {"req-1"}})

			lines := logLines(t, &buf)
			if len(lines) != 1 {
				t.Fatalf("got %d log lines, want 1: %s", len(lines), buf.String())
			}
			l := lines[0]
			want := map[string]any{
				"level":      tt.wantLevel,
				"msg":        "request",
				"request_id": "req-1",
				"method":     "GET",
				"path":       tt.path,
				"status":     float64(w.Code),
				"bytes":      float64(len("hello")),
			}
			for k, v := range want {
				if l[k] != v {
					t.Errorf("%s = %v, want %v", k, l[k], v)
				}
			}
			for _, k := range []string{"duration_ms", "client_ip"} {
				if _, ok := l[k]; !ok {
					t.Errorf("missing %s", k)
				}
			}
			if strings.Contains(buf.String(), "secret") {
				t.Error("query string was logged")
			}
			errs, hasErrs := l["errors"]
			if (tt.err != nil) != hasErrs {
				t.Fatalf("errors field present = %v, want %v", hasErrs, tt.err != nil)
			}
			if tt.err != nil && !strings.Contains(strings.Join(toStrings(errs), " "), tt.err.Error()) {
				t.Errorf("errors = %v, want it to include %q", errs, tt.err)
			}
		})
	}
}

func toStrings(v any) []string {
	var out []string
	if list, ok := v.([]any); ok {
		for _, s := range list {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
	}
	return out
}

func TestRecovery(t *testing.T) {
	var buf bytes.Buffer
	r := newEngine(newLogger(&buf))
	r.GET("/boom", func(*gin.Context) { panic("kaboom") })

	w := get(r, "/boom", http.Header{HeaderRequestID: {"req-2"}})

	if w.Code != http.StatusInternalServerError || w.Body.String() != "custom 500" {
		t.Errorf("got %d %q, want 500 from the respond func", w.Code, w.Body.String())
	}
	lines := logLines(t, &buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want panic + request: %s", len(lines), buf.String())
	}
	p, req := lines[0], lines[1]
	if p["msg"] != "panic" || p["panic"] != "kaboom" || p["request_id"] != "req-2" {
		t.Errorf("panic log = %v", p)
	}
	if stack, _ := p["stack"].(string); !strings.Contains(stack, "middleware_test.go") {
		t.Errorf("panic log has no stack pointing at the handler: %q", stack)
	}
	if req["level"] != "ERROR" || req["status"] != float64(500) {
		t.Errorf("request log = %v, want an ERROR line with status 500", req)
	}
	if !strings.Contains(strings.Join(toStrings(req["errors"]), " "), "panic: kaboom") {
		t.Errorf("request log errors = %v, want the panic", req["errors"])
	}
}
