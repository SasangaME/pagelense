package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/middleware"
)

var _ Analyzer = (*analyzer.Analyzer)(nil)

var discardLogger = slog.New(slog.DiscardHandler)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type fakeAnalyzer struct {
	result       analyzer.Result
	err          error
	gotURL       string
	gotRequestID string
	called       bool
}

// Analyze records its inputs and, like the real analyzer, returns the context error once the request is canceled.
func (f *fakeAnalyzer) Analyze(ctx context.Context, rawURL string) (analyzer.Result, error) {
	f.called = true
	f.gotURL = rawURL
	f.gotRequestID = middleware.RequestIDFrom(ctx)
	if err := ctx.Err(); err != nil {
		return analyzer.Result{}, err
	}
	return f.result, f.err
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	h, err := New(&fakeAnalyzer{}, discardLogger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func serve(h http.Handler, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestRoutes(t *testing.T) {
	h := newTestHandler(t)
	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/", http.StatusNotFound}, // pages are served by the frontend
		{http.MethodPost, "/analyze", http.StatusNotFound},
		{http.MethodGet, "/api/v1/analyze", http.StatusMethodNotAllowed},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			if got := serve(h, tt.method, tt.path).Code; got != tt.want {
				t.Errorf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRoutesRegistered(t *testing.T) {
	h := newTestHandler(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/analyze"},
		{http.MethodGet, "/healthz"},
	} {
		code := serve(h, route.method, route.path).Code
		if code == http.StatusNotFound || code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status %d, route not registered", route.method, route.path, code)
		}
	}
}

type panicAnalyzer struct{}

func (panicAnalyzer) Analyze(context.Context, string) (analyzer.Result, error) {
	panic("analyzer bug")
}

func TestPanicReturnsJSONError(t *testing.T) {
	w := postJSON(newAPIHandler(t, panicAnalyzer{}), `{"url": "https://example.com"}`)
	decodeError(t, w, http.StatusInternalServerError)
	if w.Header().Get(middleware.HeaderRequestID) == "" {
		t.Error("no request ID on the panic response")
	}
}

func TestExtraMiddlewareSeesFinalStatus(t *testing.T) {
	var got int
	record := func(c *gin.Context) {
		c.Next()
		got = c.Writer.Status()
	}
	h, err := New(panicAnalyzer{}, discardLogger, record)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	postJSON(h, `{"url": "https://example.com"}`)

	// extra middleware wraps recovery, so a recovered panic is seen as the 500 it was answered with
	if got != http.StatusInternalServerError {
		t.Errorf("middleware saw status %d, want 500", got)
	}
}
