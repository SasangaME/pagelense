package analyzer

import (
	"errors"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "https", raw: "https://example.com", want: "https://example.com"},
		{name: "http with path and query", raw: "http://example.com/a/b?q=1", want: "http://example.com/a/b?q=1"},
		{name: "surrounding whitespace", raw: "  https://example.com/  ", want: "https://example.com/"},
		{name: "uppercase scheme and host", raw: "HTTPS://Example.COM/Path", want: "https://example.com/Path"},
		{name: "fragment dropped", raw: "https://example.com/page#section", want: "https://example.com/page"},
		{name: "port kept", raw: "http://example.com:8080/", want: "http://example.com:8080/"},
		{name: "ipv6 host", raw: "http://[2001:db8::1]:8080/", want: "http://[2001:db8::1]:8080/"},

		{name: "empty", raw: "", wantErr: true},
		{name: "only whitespace", raw: "   ", wantErr: true},
		{name: "no scheme", raw: "example.com", wantErr: true},
		{name: "host with port but no scheme", raw: "example.com:8080", wantErr: true},
		{name: "ftp scheme", raw: "ftp://example.com", wantErr: true},
		{name: "javascript scheme", raw: "javascript:alert(1)", wantErr: true},
		{name: "mailto scheme", raw: "mailto:someone@example.com", wantErr: true},
		{name: "scheme only", raw: "https://", wantErr: true},
		{name: "path without host", raw: "https:///path", wantErr: true},
		{name: "port without host", raw: "https://:8080", wantErr: true},
		{name: "opaque http", raw: "http:example.com", wantErr: true},
		{name: "unclosed ipv6 bracket", raw: "http://[::1", wantErr: true},
		{name: "space in host", raw: "http://exa mple.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseURL(tt.raw)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidURL) {
					t.Fatalf("parseURL(%q) error = %v, want ErrInvalidURL", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseURL(%q) unexpected error: %v", tt.raw, err)
			}
			if got.String() != tt.want {
				t.Errorf("parseURL(%q) = %q, want %q", tt.raw, got.String(), tt.want)
			}
		})
	}
}

func TestParseURLMessages(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"", "invalid URL: URL is required"},
		{"example.com", "invalid URL: must start with http:// or https://"},
		{"https://", "invalid URL: missing host"},
		{"http://exa mple.com", `invalid URL: invalid character " " in host name`},
		{"http://[::1", "invalid URL: missing ']' in host"},
		{"http://example.com:abc/", `invalid URL: invalid port ":abc" after host`},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			_, err := parseURL(tt.raw)
			if err == nil || err.Error() != tt.want {
				t.Errorf("parseURL(%q) error = %v, want %q", tt.raw, err, tt.want)
			}
		})
	}
}
