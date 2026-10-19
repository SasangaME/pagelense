package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SasangaME/pagelense/internal/analyzer"
)

func postJSON(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func newAPIHandler(t *testing.T, a Analyzer) http.Handler {
	t.Helper()
	h, err := New(a)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

// decodeError checks that w is a JSON error body whose status matches the HTTP status.
func decodeError(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) errorDetail {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body %s", w.Code, wantStatus, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", w.Body, err)
	}
	if body.Error.Status != wantStatus {
		t.Errorf("body status = %d, want %d", body.Error.Status, wantStatus)
	}
	if body.Error.Message == "" {
		t.Error("empty error message")
	}
	return body.Error
}

func TestAnalyzeOK(t *testing.T) {
	want := analyzer.Result{
		URL:          "https://example.com/home",
		HTMLVersion:  "HTML5",
		Title:        "Example",
		Headings:     analyzer.Headings{H1: 1, H2: 3},
		Links:        analyzer.LinkStats{Internal: 5, External: 2, Checked: 7, Inaccessible: 1},
		HasLoginForm: true,
	}
	fake := &fakeAnalyzer{result: want}
	w := postJSON(newAPIHandler(t, fake), `{"url": "https://example.com"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body)
	}
	if fake.gotURL != "https://example.com" {
		t.Errorf("analyzer got %q, want the requested URL", fake.gotURL)
	}
	var got analyzer.Result
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got != want {
		t.Errorf("result = %+v, want %+v", got, want)
	}
}

func TestAnalyzeBadRequest(t *testing.T) {
	tests := []struct{ name, body string }{
		{"empty body", ""},
		{"malformed", `{"url": `},
		{"not an object", `"https://example.com"`},
		{"wrong type", `{"url": 42}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeAnalyzer{}
			decodeError(t, postJSON(newAPIHandler(t, fake), tt.body), http.StatusBadRequest)
			if fake.called {
				t.Error("analyzer called for a bad request")
			}
		})
	}
}

func TestAnalyzeBodyTooLarge(t *testing.T) {
	body := `{"url": "https://example.com/` + strings.Repeat("a", maxRequestBytes) + `"}`
	fake := &fakeAnalyzer{}
	decodeError(t, postJSON(newAPIHandler(t, fake), body), http.StatusRequestEntityTooLarge)
	if fake.called {
		t.Error("analyzer called for an oversized body")
	}
}

func TestAnalyzeMissingURLIsPassedOn(t *testing.T) {
	// an empty or missing url is the analyzer's to reject, so the API and the web form report it the same way
	fake := &fakeAnalyzer{err: analyzer.ErrInvalidURL}
	w := postJSON(newAPIHandler(t, fake), `{}`)
	if !fake.called || fake.gotURL != "" {
		t.Errorf("called = %v, gotURL = %q; want the analyzer called with an empty URL", fake.called, fake.gotURL)
	}
	status, _ := errorResponse(fake.err)
	decodeError(t, w, status)
}

func TestAnalyzeError(t *testing.T) {
	fake := &fakeAnalyzer{err: errors.New("boom")}
	w := postJSON(newAPIHandler(t, fake), `{"url": "https://example.com"}`)

	status, msg := errorResponse(fake.err)
	if got := decodeError(t, w, status); got.Message != msg {
		t.Errorf("message = %q, want %q", got.Message, msg)
	}
}
