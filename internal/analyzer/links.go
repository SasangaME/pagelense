package analyzer

import (
	"net/url"
	"strings"
)

// links holds a page's http and https links as absolute URLs, split by host.
type links struct {
	internal []string
	external []string
}

// classifyLinks resolves each href against the page URL (or a valid <base href>) and splits them into internal and external by host.
func classifyLinks(pageURL *url.URL, baseHref string, hrefs []string) links {
	base := pageURL
	if u, ok := resolveLink(pageURL, baseHref); ok {
		base = u
	}

	host := strings.ToLower(pageURL.Hostname())
	var l links
	for _, href := range hrefs {
		u, ok := resolveLink(base, href)
		if !ok {
			continue
		}
		if u.Hostname() == host {
			l.internal = append(l.internal, u.String())
		} else {
			l.external = append(l.external, u.String())
		}
	}
	return l
}

// resolveLink returns href as an absolute http(s) URL with a lowercase host and no fragment; empty, fragment-only and non-http(s) hrefs are skipped.
func resolveLink(base *url.URL, href string) (*url.URL, bool) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return nil, false
	}

	ref, err := url.Parse(href)
	if err != nil {
		return nil, false
	}
	u := base.ResolveReference(ref)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}
	if u.Hostname() == "" {
		return nil, false
	}

	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	return u, true
}
