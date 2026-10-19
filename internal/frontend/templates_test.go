package frontend

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/SasangaME/pagelense/internal/analyzer"
)

// These tests render the real templates in web/, so they check the markup the browser gets.

func newRealHandler(t *testing.T, a Analyzer) http.Handler {
	t.Helper()
	h, err := New(a, discardLogger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func assertContains(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("page is missing %q", w)
		}
	}
}

// assertOnlyAppScript checks the page loads app.js and has no other script, inline or external.
func assertOnlyAppScript(t *testing.T, body string) {
	t.Helper()
	if n := strings.Count(strings.ToLower(body), "<script"); n != 1 {
		t.Errorf("page has %d <script> tags, want 1", n)
	}
	assertContains(t, body, `<script src="/static/app.js" defer></script>`)
}

func TestIndexPage(t *testing.T) {
	w := serve(newRealHandler(t, &fakeAnalyzer{}), http.MethodGet, "/")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	assertContains(t, body,
		"<!DOCTYPE html>",
		`<link rel="stylesheet" href="/static/style.css">`,
		`<form class="analyze" method="post" action="/analyze">`,
		`name="url"`,
		`<button type="submit">Analyze</button>`,
	)
	assertOnlyAppScript(t, body)
}

func TestResultPage(t *testing.T) {
	fake := &fakeAnalyzer{result: analyzer.Result{
		URL:          "https://example.com/home",
		HTMLVersion:  "HTML 4.01 Strict",
		Title:        "Tom & Jerry",
		Headings:     analyzer.Headings{H1: 1, H2: 3, H6: 2},
		Links:        analyzer.LinkStats{Internal: 12, External: 4, Checked: 16, Inaccessible: 2},
		HasLoginForm: true,
	}}
	w := postForm(newRealHandler(t, fake), url.Values{"url": {"https://example.com"}})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	assertContains(t, body,
		`<a href="https://example.com/home" rel="nofollow noopener noreferrer">https://example.com/home</a>`,
		"<td>HTML 4.01 Strict</td>",
		"<td>Tom &amp; Jerry</td>",
		"<td>Yes</td>",
		"<td>1</td><td>3</td><td>0</td><td>0</td><td>0</td><td>2</td>",
		`<th scope="row">Internal</th><td>12</td>`,
		`<th scope="row">External</th><td>4</td>`,
		`<td>2 <span class="muted">of 16 checked</span></td>`,
		`value="https://example.com"`, // the form is ready for another analysis
	)
	assertOnlyAppScript(t, body)
}

func TestResultPageNoTitleNoLogin(t *testing.T) {
	fake := &fakeAnalyzer{result: analyzer.Result{URL: "https://example.com", HTMLVersion: "Unknown"}}
	body := postForm(newRealHandler(t, fake), url.Values{"url": {"https://example.com"}}).Body.String()

	assertContains(t, body, `<span class="muted">No title</span>`, "<td>No</td>", "<td>Unknown</td>")
}

func TestErrorPage(t *testing.T) {
	fake := &fakeAnalyzer{err: &APIError{Status: http.StatusNotFound, Message: "example.com returned 404 Not Found"}}
	w := postForm(newRealHandler(t, fake), url.Values{"url": {`https://example.com/missing?q="><b>`}})

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	body := w.Body.String()
	assertContains(t, body,
		`<section class="error" role="alert">`,
		"<h1>Error 404 Not Found</h1>",
		"<p>example.com returned 404 Not Found</p>",
		// the submitted URL stays in the form, escaped so it cannot break out of the attribute
		`value="https://example.com/missing?q=&#34;&gt;&lt;b&gt;"`,
	)
	if strings.Contains(body, `"><b>`) {
		t.Error("submitted URL is not escaped")
	}
	assertOnlyAppScript(t, body)
}

func TestErrorPageUnknownStatus(t *testing.T) {
	fake := &fakeAnalyzer{err: &APIError{Status: 599, Message: "odd upstream"}}
	body := postForm(newRealHandler(t, fake), url.Values{"url": {"https://example.com"}}).Body.String()

	assertContains(t, body, "<h1>Error 599</h1>")
}

func TestResultPagePartial(t *testing.T) {
	fake := &fakeAnalyzer{result: analyzer.Result{URL: "https://example.com", HTMLVersion: "HTML5"}}
	body := postPartial(newRealHandler(t, fake), url.Values{"url": {"https://example.com"}}).Body.String()

	assertContains(t, body, "<h1>Analysis</h1>", `<form class="analyze" method="post" action="/analyze">`)
	for _, layout := range []string{"<!DOCTYPE", "<header>", "<main>", "<script"} {
		if strings.Contains(body, layout) {
			t.Errorf("partial page contains layout markup %q", layout)
		}
	}
}

func TestAppScript(t *testing.T) {
	w := serve(newRealHandler(t, &fakeAnalyzer{}), http.MethodGet, "/static/app.js")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want javascript", ct)
	}
	// app.js and the server must agree on the header name
	assertContains(t, w.Body.String(), `"`+partialHeader+`": "1"`)
}

func TestStylesheet(t *testing.T) {
	w := serve(newRealHandler(t, &fakeAnalyzer{}), http.MethodGet, "/static/style.css")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("stylesheet is empty")
	}
}
