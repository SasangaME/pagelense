package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/web"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type fakeAnalyzer struct {
	result analyzer.Result
	err    error
	gotURL string
}

func (f *fakeAnalyzer) Analyze(_ context.Context, rawURL string) (analyzer.Result, error) {
	f.gotURL = rawURL
	return f.result, f.err
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	h, err := New(&fakeAnalyzer{})
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
		{http.MethodGet, "/nope", http.StatusNotFound},
		{http.MethodDelete, "/", http.StatusMethodNotAllowed},
		{http.MethodGet, "/analyze", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/analyze", http.StatusNotFound}, // served by the backend
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodGet, "/static/", http.StatusNotFound}, // no directory listing
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
		{http.MethodGet, "/"},
		{http.MethodPost, "/analyze"},
		{http.MethodGet, "/healthz"},
	} {
		code := serve(h, route.method, route.path).Code
		if code == http.StatusNotFound || code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status %d, route not registered", route.method, route.path, code)
		}
	}
}

func TestStaticCSS(t *testing.T) {
	w := serve(newTestHandler(t), http.MethodGet, "/static/style.css")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}
}

func TestParsePagesEmbedded(t *testing.T) {
	p, err := parsePages(web.FS)
	if err != nil {
		t.Fatalf("parsePages: %v", err)
	}
	for _, name := range pageNames {
		if p[name] == nil {
			t.Errorf("page %q not parsed", name)
		}
	}
}

func TestPagesRenderInsideLayout(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/layout.html": {Data: []byte(`{{define "layout"}}<main>{{template "content" .}}</main>{{end}}`)},
		"templates/index.html":  {Data: []byte(`{{define "content"}}index {{.}}{{end}}`)},
		"templates/result.html": {Data: []byte(`{{define "content"}}result {{.}}{{end}}`)},
		"templates/error.html":  {Data: []byte(`{{define "content"}}error {{.}}{{end}}`)},
	}
	p, err := parsePages(fsys)
	if err != nil {
		t.Fatalf("parsePages: %v", err)
	}
	for _, name := range pageNames {
		w := httptest.NewRecorder()
		if err := p.Instance(name, "<x>").Render(w); err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		// each page keeps its own "content", and data is HTML-escaped
		if want := "<main>" + name + " &lt;x&gt;</main>"; w.Body.String() != want {
			t.Errorf("render %s = %q, want %q", name, w.Body.String(), want)
		}
	}
}

func TestParsePagesErrors(t *testing.T) {
	layout := &fstest.MapFile{Data: []byte(`{{define "layout"}}{{template "content" .}}{{end}}`)}
	page := &fstest.MapFile{Data: []byte(`{{define "content"}}ok{{end}}`)}
	tests := []struct {
		name string
		fsys fstest.MapFS
	}{
		{"missing page", fstest.MapFS{
			"templates/layout.html": layout,
			"templates/index.html":  page,
			"templates/result.html": page,
		}},
		{"missing layout", fstest.MapFS{
			"templates/index.html":  page,
			"templates/result.html": page,
			"templates/error.html":  page,
		}},
		{"syntax error", fstest.MapFS{
			"templates/layout.html": layout,
			"templates/index.html":  {Data: []byte(`{{define "content"}}{{.Missing}`)},
			"templates/result.html": page,
			"templates/error.html":  page,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parsePages(tt.fsys); err == nil {
				t.Error("expected error")
			}
		})
	}
}
