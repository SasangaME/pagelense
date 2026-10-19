package analyzer

import (
	"strings"
	"testing"
)

func TestLoginFieldDetection(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{name: "no forms", html: `<p>Hello</p>`, want: false},
		{
			name: "username and password",
			html: `<form><input type="text" name="user"><input type="password" name="pass"></form>`,
			want: true,
		},
		{name: "password only", html: `<form><input type="password"></form>`, want: true},
		{name: "uppercase password type", html: `<form><input type="PASSWORD"></form>`, want: true},
		{name: "password nested deep in form", html: `<form><div><fieldset><input type="password"></fieldset></div></form>`, want: true},
		{
			name: "login form among other forms",
			html: `<form><input type="search"></form><form><input type="password"></form>`,
			want: true,
		},
		{
			name: "identifier-first step",
			html: `<form><input type="email" autocomplete="username"><button>Next</button></form>`,
			want: true,
		},
		{
			name: "autocomplete username among other tokens",
			html: `<form><input type="text" autocomplete="section-login Username webauthn"></form>`,
			want: true,
		},
		{name: "email only", html: `<form><input type="email" name="email"></form>`, want: false},
		{name: "text input named username", html: `<form><input type="text" name="username"></form>`, want: false},
		{name: "autocomplete email", html: `<form><input type="email" autocomplete="email"></form>`, want: false},
		{name: "autocomplete token is not a substring match", html: `<form><input autocomplete="nickname"></form>`, want: false},
		{name: "password outside any form", html: `<div><input type="password"></div>`, want: true},
		{
			name: "formless identifier-first step",
			html: `<div><input type="text" name="identifier" autocomplete="username webauthn"><div role="button">Next</div></div>`,
			want: true,
		},
		{name: "search input outside any form", html: `<div><input type="search"></div>`, want: false},
		{name: "password type on non-input element", html: `<form><button type="password">Go</button></form>`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parsePage(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("parsePage: unexpected error: %v", err)
			}
			if got := p.hasLogin; got != tt.want {
				t.Errorf("hasLogin = %v, want %v", got, tt.want)
			}
		})
	}
}
