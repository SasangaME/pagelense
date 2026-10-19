package analyzer

import (
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSamples(t *testing.T) {
	tests := []struct {
		file         string
		pageURL      string
		wantVersion  string
		wantTitle    string
		wantHeadings Headings
		wantInternal []string
		wantExternal []string
		wantLogin    bool
	}{
		{
			file:         "html5_page.html",
			pageURL:      "https://example.com/store",
			wantVersion:  "HTML5",
			wantTitle:    "Example Store",
			wantHeadings: Headings{H1: 1, H2: 2, H3: 2, H4: 1, H5: 1, H6: 1},
			wantExternal: []string{
				"https://cdn.example.com/",
				"https://cdn.example.com/products",
				"https://blog.example.org/",
			},
			wantLogin: true,
		},
		{
			file:         "legacy_html401.html",
			pageURL:      "http://oldcorp.example/index.html",
			wantVersion:  "HTML 4.01 Transitional",
			wantTitle:    "Old Corp Intranet",
			wantHeadings: Headings{H1: 1, H2: 1},
			wantInternal: []string{"http://oldcorp.example/news.html"},
			wantExternal: []string{"http://www.oldcorp.example/about.html"},
			wantLogin:    true,
		},
		{
			file:         "xhtml_strict.html",
			pageURL:      "https://docs.example.com/",
			wantVersion:  "XHTML 1.0 Strict",
			wantTitle:    "XHTML Docs",
			wantHeadings: Headings{H1: 1, H2: 2, H3: 1},
			wantInternal: []string{"https://docs.example.com/docs/"},
			wantExternal: []string{"https://www.w3.org/TR/xhtml1/"},
		},
		{
			file:         "no_doctype.html",
			pageURL:      "https://example.com/start/",
			wantVersion:  "Unknown",
			wantHeadings: Headings{H1: 1},
			wantInternal: []string{"https://example.com/start/page2.html"},
		},
		{
			file:         "links_page.html",
			pageURL:      "https://example.com/blog/post.html",
			wantVersion:  "HTML5",
			wantTitle:    "Link Cases",
			wantHeadings: Headings{H1: 1},
			wantInternal: []string{
				"https://example.com/",
				"https://example.com/blog/related.html",
				"https://example.com/about/",
				"https://example.com/contact",
				"http://example.com/legacy",
				"https://example.com/docs",
				"https://example.com/",
			},
			wantExternal: []string{
				"https://www.example.com/",
				"https://github.com/golang/go",
				"https://cdn.example.net/app.js",
			},
		},
		{
			file:         "identifier_first_login.html",
			pageURL:      "https://accounts.example.com/signin",
			wantVersion:  "HTML5",
			wantTitle:    "Sign in",
			wantHeadings: Headings{H1: 1},
			wantInternal: []string{"https://accounts.example.com/signup"},
			wantLogin:    true,
		},
		{
			file:         "newsletter_signup.html",
			pageURL:      "https://example.com/newsletter",
			wantVersion:  "HTML5",
			wantTitle:    "Newsletter",
			wantHeadings: Headings{H1: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			pageURL, err := url.Parse(tt.pageURL)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tt.pageURL, err)
			}

			p, err := parsePage(f)
			if err != nil {
				t.Fatalf("parsePage: unexpected error: %v", err)
			}
			l := classifyLinks(pageURL, p.baseHref, p.hrefs)

			if p.htmlVersion != tt.wantVersion {
				t.Errorf("htmlVersion = %q, want %q", p.htmlVersion, tt.wantVersion)
			}
			if p.title != tt.wantTitle {
				t.Errorf("title = %q, want %q", p.title, tt.wantTitle)
			}
			if p.headings != tt.wantHeadings {
				t.Errorf("headings = %+v, want %+v", p.headings, tt.wantHeadings)
			}
			if !slices.Equal(l.internal, tt.wantInternal) {
				t.Errorf("internal = %q, want %q", l.internal, tt.wantInternal)
			}
			if !slices.Equal(l.external, tt.wantExternal) {
				t.Errorf("external = %q, want %q", l.external, tt.wantExternal)
			}
			if got := p.hasLogin; got != tt.wantLogin {
				t.Errorf("hasLogin = %v, want %v", got, tt.wantLogin)
			}
		})
	}
}
