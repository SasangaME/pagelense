package analyzer

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/SasangaME/pagelense/internal/fetcher"
	"github.com/SasangaME/pagelense/internal/linkcheck"
)

var (
	_ Fetcher     = (*fetcher.Fetcher)(nil)
	_ LinkChecker = (*linkcheck.Checker)(nil)
)

type fakeFetcher struct {
	page   *fetcher.Page
	err    error
	gotURL string
	calls  int
}

func (f *fakeFetcher) Fetch(_ context.Context, rawURL string) (*fetcher.Page, error) {
	f.calls++
	f.gotURL = rawURL
	return f.page, f.err
}

type fakeChecker struct {
	statuses map[string]int
	limit    int
	cancel   context.CancelFunc
	gotURLs  []string
}

func (c *fakeChecker) Check(_ context.Context, urls []string) []linkcheck.Result {
	c.gotURLs = urls
	if c.cancel != nil {
		c.cancel()
		return []linkcheck.Result{{URL: urls[0], Err: context.Canceled}}
	}

	var out []linkcheck.Result
	seen := make(map[string]bool)
	for _, u := range urls {
		if seen[u] || (c.limit > 0 && len(out) == c.limit) {
			continue
		}
		seen[u] = true
		status, ok := c.statuses[u]
		if !ok {
			status = http.StatusOK
		}
		out = append(out, linkcheck.Result{URL: u, StatusCode: status})
	}
	return out
}

func htmlPage(t *testing.T, rawURL, body string) *fetcher.Page {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return &fetcher.Page{URL: u, Body: []byte(body)}
}

const fullPage = `<!DOCTYPE html>
<html><head><title> Shop  Home </title></head>
<body>
<h1>Shop</h1><h2>A</h2><h2>B</h2><h3>C</h3>
<a href="/products">Products</a>
<a href="/products">Products again</a>
<a href="/missing">Missing</a>
<a href="https://partner.example.org/">Partner</a>
<a href="https://down.example.net/">Down</a>
<a href="mailto:hi@example.com">Mail</a>
<a href="#top">Top</a>
<form action="/login"><input name="user"><input type="password" name="pw"></form>
</body></html>`

func TestAnalyze(t *testing.T) {
	f := &fakeFetcher{page: htmlPage(t, "https://example.com/", fullPage)}
	c := &fakeChecker{statuses: map[string]int{
		"https://example.com/missing":  http.StatusNotFound,
		"https://down.example.net/":    http.StatusServiceUnavailable,
		"https://partner.example.org/": http.StatusOK,
	}}

	got, err := New(f, c).Analyze(t.Context(), "  https://EXAMPLE.com/#intro ")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if f.gotURL != "https://example.com/" {
		t.Errorf("fetched %q, want the normalized URL https://example.com/", f.gotURL)
	}
	wantChecked := []string{
		"https://example.com/products",
		"https://example.com/products",
		"https://example.com/missing",
		"https://partner.example.org/",
		"https://down.example.net/",
	}
	if !slices.Equal(c.gotURLs, wantChecked) {
		t.Errorf("checked URLs = %q, want %q", c.gotURLs, wantChecked)
	}

	want := Result{
		URL:          "https://example.com/",
		HTMLVersion:  "HTML5",
		Title:        "Shop Home",
		Headings:     Headings{H1: 1, H2: 2, H3: 1},
		Links:        LinkStats{Internal: 3, External: 2, Checked: 5, Inaccessible: 2},
		HasLoginForm: true,
	}
	if got != want {
		t.Errorf("Analyze() = %+v\nwant        %+v", got, want)
	}
}

func TestAnalyzeInvalidURL(t *testing.T) {
	for _, raw := range []string{"", "example.com", "ftp://example.com/", "https://"} {
		t.Run(raw, func(t *testing.T) {
			f := &fakeFetcher{}
			_, err := New(f, &fakeChecker{}).Analyze(t.Context(), raw)
			if !errors.Is(err, ErrInvalidURL) {
				t.Errorf("err = %v, want ErrInvalidURL", err)
			}
			if f.calls != 0 {
				t.Errorf("fetcher called %d times for invalid input", f.calls)
			}
		})
	}
}

func TestAnalyzeFetchError(t *testing.T) {
	fetchErr := &fetcher.FetchError{StatusCode: http.StatusNotFound, Message: "example.com returned 404 Not Found"}
	c := &fakeChecker{}

	_, err := New(&fakeFetcher{err: fetchErr}, c).Analyze(t.Context(), "https://example.com/")

	var fe *fetcher.FetchError
	if !errors.As(err, &fe) || fe.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want the *fetcher.FetchError with status 404", err)
	}
	if c.gotURLs != nil {
		t.Error("links were checked after the fetch failed")
	}
}

func TestAnalyzeRedirect(t *testing.T) {
	body := `<!DOCTYPE html><a href="/about">About</a><a href="https://example.com/">Old host</a>`
	f := &fakeFetcher{page: htmlPage(t, "https://www.example.com/home/", body)}

	got, err := New(f, &fakeChecker{}).Analyze(t.Context(), "https://example.com/")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if got.URL != "https://www.example.com/home/" {
		t.Errorf("URL = %q, want the final URL after redirects", got.URL)
	}
	if got.Links.Internal != 1 || got.Links.External != 1 {
		t.Errorf("links = %+v, want 1 internal (/about on www) and 1 external (the old host)", got.Links)
	}
}

func TestAnalyzeLinkCap(t *testing.T) {
	body := `<!DOCTYPE html>
<a href="/a">a</a><a href="/b">b</a><a href="/a">a again</a><a href="/c">c</a>`
	f := &fakeFetcher{page: htmlPage(t, "https://example.com/", body)}
	c := &fakeChecker{limit: 2, statuses: map[string]int{"https://example.com/a": http.StatusGone}}

	got, err := New(f, c).Analyze(t.Context(), "https://example.com/")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	want := LinkStats{Internal: 4, Checked: 3, Inaccessible: 2}
	if got.Links != want {
		t.Errorf("links = %+v, want %+v", got.Links, want)
	}
}

func TestAnalyzeNoLinks(t *testing.T) {
	f := &fakeFetcher{page: htmlPage(t, "https://example.com/", `<!DOCTYPE html><title>Empty</title>`)}

	got, err := New(f, &fakeChecker{}).Analyze(t.Context(), "https://example.com/")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.Links != (LinkStats{}) {
		t.Errorf("links = %+v, want all zero", got.Links)
	}
}

func TestAnalyzeCanceledDuringLinkCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f := &fakeFetcher{page: htmlPage(t, "https://example.com/", `<a href="/a">a</a><a href="/b">b</a>`)}

	_, err := New(f, &fakeChecker{cancel: cancel}).Analyze(ctx, "https://example.com/")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled instead of a partial result", err)
	}
}
