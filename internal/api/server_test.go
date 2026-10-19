package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/analyzer"
)

var _ Analyzer = (*analyzer.Analyzer)(nil)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type fakeAnalyzer struct {
	result analyzer.Result
	err    error
	gotURL string
	called bool
}

func (f *fakeAnalyzer) Analyze(_ context.Context, rawURL string) (analyzer.Result, error) {
	f.called = true
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
