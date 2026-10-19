package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/fetcher"
	"github.com/SasangaME/pagelense/internal/middleware"
)

// assertJSON compares JSON documents by value, so key order and spacing don't matter but every field name does.
func assertJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("response is not JSON: %q", got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad expected JSON: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("JSON = %s\nwant   %s", got, want)
	}
}

// The frontend decodes these bodies, so the field names are a contract: these tests fail if a json tag changes.
func TestContractSuccessJSON(t *testing.T) {
	fake := &fakeAnalyzer{result: analyzer.Result{
		URL:          "https://example.com/home",
		HTMLVersion:  "HTML5",
		Title:        "Example",
		Headings:     analyzer.Headings{H1: 1, H2: 3},
		Links:        analyzer.LinkStats{Internal: 5, External: 2, Checked: 7, Inaccessible: 1},
		HasLoginForm: true,
	}}
	w := postJSON(newAPIHandler(t, fake), `{"url": "https://example.com"}`)

	assertJSON(t, w.Body.Bytes(), `{
		"url": "https://example.com/home",
		"html_version": "HTML5",
		"title": "Example",
		"headings": {"h1": 1, "h2": 3, "h3": 0, "h4": 0, "h5": 0, "h6": 0},
		"links": {"internal": 5, "external": 2, "checked": 7, "inaccessible": 1},
		"has_login_form": true
	}`)
}

func TestContractErrorJSON(t *testing.T) {
	fake := &fakeAnalyzer{err: fetchErr(http.StatusNotFound, "example.com returned 404 Not Found")}
	w := postJSON(newAPIHandler(t, fake), `{"url": "https://example.com/missing"}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	assertJSON(t, w.Body.Bytes(), `{"error": {"status": 404, "message": "example.com returned 404 Not Found"}}`)
}

func TestAnalyzerGetsRequestContext(t *testing.T) {
	fake := &fakeAnalyzer{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", strings.NewReader(`{"url": "https://example.com"}`))
	req.Header.Set(middleware.HeaderRequestID, "from-frontend-1")
	w := httptest.NewRecorder()
	newAPIHandler(t, fake).ServeHTTP(w, req)

	// the ID the frontend forwards reaches the analyzer and comes back, so logs from both services line up
	if fake.gotRequestID != "from-frontend-1" {
		t.Errorf("analyzer saw request ID %q, want the incoming one", fake.gotRequestID)
	}
	if got := w.Header().Get(middleware.HeaderRequestID); got != "from-frontend-1" {
		t.Errorf("response X-Request-ID = %q, want the incoming one", got)
	}
}

func TestRequestIDOnEveryResponse(t *testing.T) {
	tests := []struct {
		name string
		fake *fakeAnalyzer
		body string
	}{
		{"success", &fakeAnalyzer{}, `{"url": "https://example.com"}`},
		{"bad request", &fakeAnalyzer{}, `{`},
		{"analysis error", &fakeAnalyzer{err: analyzer.ErrInvalidURL}, `{"url": "x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postJSON(newAPIHandler(t, tt.fake), tt.body)
			if w.Header().Get(middleware.HeaderRequestID) == "" {
				t.Errorf("status %d response has no X-Request-ID", w.Code)
			}
		})
	}
}

func TestCanceledRequestReachesAnalyzer(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the client has gone away
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/analyze", strings.NewReader(`{"url": "https://example.com"}`))
	w := httptest.NewRecorder()
	newAPIHandler(t, &fakeAnalyzer{}).ServeHTTP(w, req)

	if w.Code != statusClientClosedRequest {
		t.Errorf("status = %d, want %d", w.Code, statusClientClosedRequest)
	}
}

func TestCausesAreLoggedNotReturned(t *testing.T) {
	cause := errors.New("dial tcp 10.0.0.5:443: connect: connection refused")
	fake := &fakeAnalyzer{err: &fetcher.FetchError{
		StatusCode: http.StatusBadGateway,
		Message:    "connection refused by example.com",
		Err:        cause,
	}}
	var logs bytes.Buffer
	h, err := New(fake, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	w := postJSON(h, `{"url": "https://example.com"}`)

	assertJSON(t, w.Body.Bytes(), `{"error": {"status": 502, "message": "connection refused by example.com"}}`)
	if strings.Contains(w.Body.String(), "10.0.0.5") {
		t.Errorf("response leaks the internal cause: %s", w.Body)
	}
	if !strings.Contains(logs.String(), "10.0.0.5") {
		t.Errorf("log is missing the cause: %s", logs.String())
	}
}
