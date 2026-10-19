package frontend

import (
	"net/http"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	fake := &fakeAnalyzer{err: &APIError{Status: http.StatusBadGateway, Message: "unavailable"}}
	w := serve(newWebHandler(t, fake), http.MethodGet, "/healthz")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got, want := w.Body.String(), `{"status":"ok"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	if fake.called {
		t.Error("health check called the analyzer")
	}
}
