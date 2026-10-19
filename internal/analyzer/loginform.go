package analyzer

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// autocomplete="username" catches sign-in pages that ask for the password on a later step.
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
