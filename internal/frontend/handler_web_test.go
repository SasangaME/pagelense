package frontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/SasangaME/pagelense/internal/analyzer"
)

// testPages print the page data as plain text, so tests check what handlers pass rather than the real markup.
var testPages = fstest.MapFS{
	"templates/layout.html": {Data: []byte(`{{define "layout"}}url={{.URL}}|{{template "content" .}}{{end}}`)},
	"templates/index.html":  {Data: []byte(`{{define "content"}}index{{end}}`)},
	"templates/result.html": {Data: []byte(`{{define "content"}}result|{{with .Result}}` +
		`{{.URL}}|{{.HTMLVersion}}|{{.Title}}|h1={{.Headings.H1}},h2={{.Headings.H2}}|` +
		`int={{.Links.Internal}},ext={{.Links.External}},checked={{.Links.Checked}},bad={{.Links.Inaccessible}}|` +
		`login={{.HasLoginForm}}{{end}}{{end}}`)},
	"templates/error.html": {Data: []byte(`{{define "content"}}error|{{.Status}}|{{.Error}}{{end}}`)},
}

func newWebHandler(t *testing.T, a Analyzer) http.Handler {
	t.Helper()
	h, err := newHandler(a, testPages, discardLogger)
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	return h
}

func postForm(h http.Handler, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/analyze", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestIndex(t *testing.T) {
	w := serve(newWebHandler(t, &fakeAnalyzer{}), http.MethodGet, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got, want := w.Body.String(), "url=|index"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestAnalyzeResult(t *testing.T) {
	fake := &fakeAnalyzer{result: analyzer.Result{
		URL:          "https://example.com/home",
		HTMLVersion:  "HTML5",
		Title:        "Example",
		Headings:     analyzer.Headings{H1: 1, H2: 3},
		Links:        analyzer.LinkStats{Internal: 5, External: 2, Checked: 7, Inaccessible: 1},
		HasLoginForm: true,
	}}
	w := postForm(newWebHandler(t, fake), url.Values{"url": {"https://example.com"}})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if fake.gotURL != "https://example.com" {
		t.Errorf("analyzer got %q, want the submitted URL", fake.gotURL)
	}
	want := "url=https://example.com|result|https://example.com/home|HTML5|Example|h1=1,h2=3|" +
		"int=5,ext=2,checked=7,bad=1|login=true"
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestAnalyzeErrorKeepsURL(t *testing.T) {
	fake := &fakeAnalyzer{err: errors.New("boom")}
	w := postForm(newWebHandler(t, fake), url.Values{"url": {"https://example.com/<x>"}})

	status, msg := errorResponse(fake.err)
	if w.Code != status {
		t.Fatalf("status = %d, want %d", w.Code, status)
	}
	// the URL is HTML-escaped, so user input cannot inject markup
	want := fmt.Sprintf("url=https://example.com/&lt;x&gt;|error|%d|%s", status, msg)
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

type panicAnalyzer struct{}

func (panicAnalyzer) Analyze(context.Context, string) (analyzer.Result, error) {
	panic("client bug")
}

func TestAnalyzePanicRendersErrorPage(t *testing.T) {
	w := postForm(newWebHandler(t, panicAnalyzer{}), url.Values{"url": {"https://example.com"}})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	want := "url=https://example.com|error|500|Something went wrong. Please try again."
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
