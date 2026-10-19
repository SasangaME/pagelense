package analyzer

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// isLoginField reports whether n is a password input or an autocomplete="username" input (identifier-first sign-in), even outside a <form>, as script-driven pages like Google's use.
func isLoginField(n *html.Node) bool {
	if n.Type != html.ElementNode || n.DataAtom != atom.Input {
		return false
	}
	if typ, _ := attr(n, "type"); strings.EqualFold(strings.TrimSpace(typ), "password") {
		return true
	}
	ac, _ := attr(n, "autocomplete")
	return slices.Contains(strings.Fields(strings.ToLower(ac)), "username")
}
