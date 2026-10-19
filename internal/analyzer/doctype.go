package analyzer

import (
	"strings"

	"golang.org/x/net/html"
)

const (
	versionHTML5   = "HTML5"
	versionUnknown = "Unknown"
)

var legacyDoctypes = []struct {
	publicID string
	version  string
}{
	{"-//W3C//DTD HTML 4.01//EN", "HTML 4.01 Strict"},
	{"-//W3C//DTD HTML 4.01 Transitional//EN", "HTML 4.01 Transitional"},
	{"-//W3C//DTD HTML 4.01 Frameset//EN", "HTML 4.01 Frameset"},
	{"-//W3C//DTD XHTML 1.0 Strict//EN", "XHTML 1.0 Strict"},
	{"-//W3C//DTD XHTML 1.0 Transitional//EN", "XHTML 1.0 Transitional"},
	{"-//W3C//DTD XHTML 1.0 Frameset//EN", "XHTML 1.0 Frameset"},
	{"-//W3C//DTD XHTML 1.1//EN", "XHTML 1.1"},
}

func htmlVersion(doc *html.Node) string {
	for n := range doc.ChildNodes() {
		if n.Type == html.DoctypeNode {
			return doctypeVersion(n)
		}
	}
	return versionUnknown
}

func doctypeVersion(n *html.Node) string {
	if n.Data != "html" {
		return versionUnknown
	}

	var public, system string
	for _, a := range n.Attr {
		switch a.Key {
		case "public":
			public = strings.TrimSpace(a.Val)
		case "system":
			system = strings.TrimSpace(a.Val)
		}
	}

	if public == "" && (system == "" || system == "about:legacy-compat") {
		return versionHTML5
	}
	for _, d := range legacyDoctypes {
		if strings.EqualFold(public, d.publicID) {
			return d.version
		}
	}
	return versionUnknown
}
