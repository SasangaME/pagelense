package analyzer

import (
	"os"
	"strings"
	"testing"
)

func TestHasLoginForm(t *testing.T) {
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
		{name: "password outside any form", html: `<div><input type="password"></div>`, want: false},
		{name: "password type on non-input element", html: `<form><button type="password">Go</button></form>`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parsePage(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("parsePage: unexpected error: %v", err)
			}
			if got := hasLoginForm(p.forms); got != tt.want {
				t.Errorf("hasLoginForm = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasLoginFormSamples(t *testing.T) {
	tests := []struct {
		file string
		want bool
	}{
		{file: "testdata/html5_page.html", want: true},
		{file: "testdata/identifier_first_login.html", want: true},
		{file: "testdata/newsletter_signup.html", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			f, err := os.Open(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			p, err := parsePage(f)
			if err != nil {
				t.Fatalf("parsePage: unexpected error: %v", err)
			}
			if got := hasLoginForm(p.forms); got != tt.want {
				t.Errorf("hasLoginForm = %v, want %v", got, tt.want)
			}
		})
	}
}
