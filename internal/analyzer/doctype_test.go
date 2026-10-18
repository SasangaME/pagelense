package analyzer

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestHTMLVersion(t *testing.T) {
	tests := []struct {
		name    string
		doctype string
		want    string
	}{
		{name: "html5", doctype: `<!DOCTYPE html>`, want: "HTML5"},
		{name: "html5 lowercase", doctype: `<!doctype html>`, want: "HTML5"},
		{name: "html5 legacy compat", doctype: `<!DOCTYPE html SYSTEM "about:legacy-compat">`, want: "HTML5"},
		{
			name:    "html 4.01 strict",
			doctype: `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd">`,
			want:    "HTML 4.01 Strict",
		},
		{
			name:    "html 4.01 transitional",
			doctype: `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">`,
			want:    "HTML 4.01 Transitional",
		},
		{
			name:    "html 4.01 frameset",
			doctype: `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Frameset//EN" "http://www.w3.org/TR/html4/frameset.dtd">`,
			want:    "HTML 4.01 Frameset",
		},
		{
			name:    "html 4.01 transitional without system id",
			doctype: `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">`,
			want:    "HTML 4.01 Transitional",
		},
		{
			name:    "xhtml 1.0 strict",
			doctype: `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">`,
			want:    "XHTML 1.0 Strict",
		},
		{
			name:    "xhtml 1.0 transitional",
			doctype: `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">`,
			want:    "XHTML 1.0 Transitional",
		},
		{
			name:    "xhtml 1.0 frameset",
			doctype: `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Frameset//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-frameset.dtd">`,
			want:    "XHTML 1.0 Frameset",
		},
		{
			name:    "xhtml 1.1",
			doctype: `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">`,
			want:    "XHTML 1.1",
		},
		{
			name:    "public id is case-insensitive",
			doctype: `<!DOCTYPE html PUBLIC "-//w3c//dtd xhtml 1.1//en">`,
			want:    "XHTML 1.1",
		},
		{name: "no doctype", doctype: ``, want: "Unknown"},
		{name: "unrecognized public id", doctype: `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">`, want: "Unknown"},
		{name: "unrecognized system id", doctype: `<!DOCTYPE html SYSTEM "http://example.com/custom.dtd">`, want: "Unknown"},
		{name: "not an html doctype", doctype: `<!DOCTYPE svg>`, want: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.doctype + "<html><head><title>t</title></head><body></body></html>"))
			if err != nil {
				t.Fatalf("html.Parse: %v", err)
			}
			if got := htmlVersion(doc); got != tt.want {
				t.Errorf("htmlVersion(%q) = %q, want %q", tt.doctype, got, tt.want)
			}
		})
	}
}
