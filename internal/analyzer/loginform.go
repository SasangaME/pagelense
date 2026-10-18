package analyzer

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// hasLoginForm reports whether any of the forms is a login form.
func hasLoginForm(forms []*html.Node) bool {
	return slices.ContainsFunc(forms, isLoginForm)
}

// isLoginForm reports whether the form has a password field, or a field marked
// autocomplete="username", which identifier-first sign-in pages use for the step before the password.
func isLoginForm(form *html.Node) bool {
	for n := range form.Descendants() {
		if n.Type != html.ElementNode || n.DataAtom != atom.Input {
			continue
		}
		if typ, _ := attr(n, "type"); strings.EqualFold(strings.TrimSpace(typ), "password") {
			return true
		}
		if ac, _ := attr(n, "autocomplete"); slices.Contains(strings.Fields(strings.ToLower(ac)), "username") {
			return true
		}
	}
	return false
}
