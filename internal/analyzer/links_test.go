package analyzer

import (
	"net/url"
	"slices"
	"testing"
)

func TestClassifyLinks(t *testing.T) {
	tests := []struct {
		name         string
		pageURL      string
		baseHref     string
		hrefs        []string
		wantInternal []string
		wantExternal []string
	}{
		{
			name:         "root-relative path",
			pageURL:      "https://example.com/docs/index.html",
			hrefs:        []string{"/about"},
			wantInternal: []string{"https://example.com/about"},
		},
		{
			name:         "relative paths",
			pageURL:      "https://example.com/docs/index.html",
			hrefs:        []string{"intro", "./setup", "../blog/"},
			wantInternal: []string{"https://example.com/docs/intro", "https://example.com/docs/setup", "https://example.com/blog/"},
		},
		{
			name:         "query kept",
			pageURL:      "https://example.com/",
			hrefs:        []string{"/search?q=go"},
			wantInternal: []string{"https://example.com/search?q=go"},
		},
		{
			name:         "fragment dropped from non-fragment-only href",
			pageURL:      "https://example.com/",
			hrefs:        []string{"/guide#install"},
			wantInternal: []string{"https://example.com/guide"},
		},
		{
			name:         "surrounding whitespace trimmed",
			pageURL:      "https://example.com/",
			hrefs:        []string{"  /about \n"},
			wantInternal: []string{"https://example.com/about"},
		},
		{
			name:    "same host is internal regardless of scheme, case or port",
			pageURL: "https://example.com/",
			hrefs:   []string{"https://example.com/a", "http://example.com/b", "https://EXAMPLE.com/c", "https://example.com:8443/d"},
			wantInternal: []string{
				"https://example.com/a",
				"http://example.com/b",
				"https://example.com/c",
				"https://example.com:8443/d",
			},
		},
		{
			name:         "different host is external",
			pageURL:      "https://example.com/",
			hrefs:        []string{"https://other.org/", "https://www.example.com/"},
			wantExternal: []string{"https://other.org/", "https://www.example.com/"},
		},
		{
			name:         "protocol-relative uses page scheme",
			pageURL:      "https://example.com/",
			hrefs:        []string{"//cdn.other.org/lib.js"},
			wantExternal: []string{"https://cdn.other.org/lib.js"},
		},
		{
			name:         "duplicates counted each time",
			pageURL:      "https://example.com/",
			hrefs:        []string{"/a", "/a"},
			wantInternal: []string{"https://example.com/a", "https://example.com/a"},
		},
		{
			name:    "non-http links skipped",
			pageURL: "https://example.com/",
			hrefs: []string{
				"", "   ", "#", "#top",
				"mailto:hello@example.com", "tel:+15551234", "javascript:void(0)", "JavaScript:alert(1)", "  javascript:x",
				"data:text/html,hi", "ftp://example.com/file",
				"http://[::1", "http:opaque",
			},
		},
		{
			name:         "absolute base href",
			pageURL:      "https://example.com/page",
			baseHref:     "https://cdn.other.org/assets/",
			hrefs:        []string{"img.png", "https://example.com/home"},
			wantInternal: []string{"https://example.com/home"},
			wantExternal: []string{"https://cdn.other.org/assets/img.png"},
		},
		{
			name:         "relative base href resolved against page URL",
			pageURL:      "https://example.com/page",
			baseHref:     "/v2/",
			hrefs:        []string{"docs"},
			wantInternal: []string{"https://example.com/v2/docs"},
		},
		{
			name:         "non-http base href ignored",
			pageURL:      "https://example.com/dir/page",
			baseHref:     "javascript:void(0)",
			hrefs:        []string{"next"},
			wantInternal: []string{"https://example.com/dir/next"},
		},
		{
			name:         "unparsable base href ignored",
			pageURL:      "https://example.com/dir/page",
			baseHref:     "http://[::1",
			hrefs:        []string{"next"},
			wantInternal: []string{"https://example.com/dir/next"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageURL, err := url.Parse(tt.pageURL)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tt.pageURL, err)
			}

			got := classifyLinks(pageURL, tt.baseHref, tt.hrefs)
			if !slices.Equal(got.internal, tt.wantInternal) {
				t.Errorf("internal = %q, want %q", got.internal, tt.wantInternal)
			}
			if !slices.Equal(got.external, tt.wantExternal) {
				t.Errorf("external = %q, want %q", got.external, tt.wantExternal)
			}
		})
	}
}
