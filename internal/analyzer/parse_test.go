package analyzer

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

func TestParsePage(t *testing.T) {
	tests := []struct {
		name         string
		html         string
		wantTitle    string
		wantHeadings Headings
		wantBase     string
		wantHrefs    []string
		wantLogin    bool
	}{
		{
			name: "empty document",
			html: ``,
		},
		{
			name:      "title",
			html:      `<title>Hello</title>`,
			wantTitle: "Hello",
		},
		{
			name:      "title whitespace collapsed",
			html:      "<title>\n  Hello \t  World\n</title>",
			wantTitle: "Hello World",
		},
		{
			name:      "first title wins",
			html:      `<title>First</title><title>Second</title>`,
			wantTitle: "First",
		},
		{
			name:      "empty first title still wins",
			html:      `<title></title><title>Second</title>`,
			wantTitle: "",
		},
		{
			name: "svg title ignored",
			html: `<body><svg><title>Icon</title></svg></body>`,
		},
		{
			name:         "headings counted per level",
			html:         `<h1>a</h1><h2>b</h2><h2>c</h2><h3>d</h3><h4>e</h4><h5>f</h5><h6>g</h6><h6>h</h6>`,
			wantHeadings: Headings{H1: 1, H2: 2, H3: 1, H4: 1, H5: 1, H6: 2},
		},
		{
			name:         "uppercase heading tags",
			html:         `<H1>a</H1><H2>b</H2>`,
			wantHeadings: Headings{H1: 1, H2: 1},
		},
		{
			name:      "anchor and area hrefs collected in order",
			html:      `<a href="/a">a</a><map><area href="/b"></map><a href="https://example.org/c">c</a>`,
			wantHrefs: []string{"/a", "/b", "https://example.org/c"},
		},
		{
			name:      "anchor without href skipped",
			html:      `<a name="x">x</a><a href="/y">y</a>`,
			wantHrefs: []string{"/y"},
		},
		{
			name:      "empty href kept",
			html:      `<a href="">self</a>`,
			wantHrefs: []string{""},
		},
		{
			name:      "svg links ignored",
			html:      `<svg><a href="/svg">x</a></svg>`,
			wantHrefs: nil,
		},
		{
			name:     "base href",
			html:     `<head><base href="https://cdn.example.com/"></head>`,
			wantBase: "https://cdn.example.com/",
		},
		{
			name:     "first base with href wins",
			html:     `<head><base target="_blank"><base href="/first/"><base href="/second/"></head>`,
			wantBase: "/first/",
		},
		{
			name:      "login field outside a form",
			html:      `<form><input type="search"></form><div><input type="password"></div>`,
			wantLogin: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parsePage(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("parsePage: unexpected error: %v", err)
			}
			if p.title != tt.wantTitle {
				t.Errorf("title = %q, want %q", p.title, tt.wantTitle)
			}
			if p.headings != tt.wantHeadings {
				t.Errorf("headings = %+v, want %+v", p.headings, tt.wantHeadings)
			}
			if p.baseHref != tt.wantBase {
				t.Errorf("baseHref = %q, want %q", p.baseHref, tt.wantBase)
			}
			if !slices.Equal(p.hrefs, tt.wantHrefs) {
				t.Errorf("hrefs = %q, want %q", p.hrefs, tt.wantHrefs)
			}
			if p.hasLogin != tt.wantLogin {
				t.Errorf("hasLogin = %v, want %v", p.hasLogin, tt.wantLogin)
			}
		})
	}
}

func TestParsePageReadError(t *testing.T) {
	readErr := errors.New("connection reset")
	_, err := parsePage(iotest.ErrReader(readErr))
	if !errors.Is(err, readErr) {
		t.Fatalf("parsePage error = %v, want it to wrap %v", err, readErr)
	}
}
