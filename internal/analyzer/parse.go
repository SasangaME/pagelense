package analyzer

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type page struct {
	htmlVersion string
	title       string
	headings    Headings
	baseHref    string
	hrefs       []string
	hasLogin    bool
}

func parsePage(r io.Reader) (page, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return page{}, fmt.Errorf("parse html: %w", err)
	}

	p := page{htmlVersion: htmlVersion(doc)}
	var titleSeen, baseSeen bool

	for n := range doc.Descendants() {
		// SVG and MathML have their own <title> and <a>
		if n.Type != html.ElementNode || n.Namespace != "" {
			continue
		}

		switch n.DataAtom {
		case atom.Title:
			if !titleSeen {
				p.title = textContent(n)
				titleSeen = true
			}
		case atom.Base:
			if href, ok := attr(n, "href"); ok && !baseSeen {
				p.baseHref = href
				baseSeen = true
			}
		case atom.H1:
			p.headings.H1++
		case atom.H2:
			p.headings.H2++
		case atom.H3:
			p.headings.H3++
		case atom.H4:
			p.headings.H4++
		case atom.H5:
			p.headings.H5++
		case atom.H6:
			p.headings.H6++
		case atom.A, atom.Area:
			if href, ok := attr(n, "href"); ok {
				p.hrefs = append(p.hrefs, href)
			}
		case atom.Input:
			if isLoginField(n) {
				p.hasLogin = true
			}
		}
	}
	return p, nil
}

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func textContent(n *html.Node) string {
	var b strings.Builder
	for c := range n.Descendants() {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
