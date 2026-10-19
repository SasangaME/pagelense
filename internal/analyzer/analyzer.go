// Package analyzer fetches a web page and reports its HTML version, title, headings, links and login form.
package analyzer

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/SasangaME/pagelense/internal/fetcher"
	"github.com/SasangaME/pagelense/internal/linkcheck"
)

// Fetcher downloads a page. *fetcher.Fetcher implements it.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (*fetcher.Page, error)
}

// LinkChecker checks whether links are reachable. *linkcheck.Checker implements it.
type LinkChecker interface {
	Check(ctx context.Context, urls []string) []linkcheck.Result
}

// Analyzer analyzes web pages. It is safe for concurrent use if its Fetcher and LinkChecker are.
type Analyzer struct {
	fetcher Fetcher
	checker LinkChecker
}

// New returns an Analyzer that downloads pages with f and checks their links with c.
func New(f Fetcher, c LinkChecker) *Analyzer {
	return &Analyzer{fetcher: f, checker: c}
}

// Analyze validates rawURL, fetches the page, parses it and checks its links.
func (a *Analyzer) Analyze(ctx context.Context, rawURL string) (Result, error) {
	u, err := parseURL(rawURL)
	if err != nil {
		return Result{}, err
	}

	pg, err := a.fetcher.Fetch(ctx, u.String())
	if err != nil {
		return Result{}, fmt.Errorf("fetch page: %w", err)
	}

	p, err := parsePage(bytes.NewReader(pg.Body))
	if err != nil {
		return Result{}, err
	}

	// links resolve against the final URL, since redirects can change the host or path
	l := classifyLinks(pg.URL, p.baseHref, p.hrefs)
	all := slices.Concat(l.internal, l.external)

	checked, inaccessible := countChecked(all, a.checker.Check(ctx, all))
	// on cancellation the results are partial and canceled checks look inaccessible, so report the error instead
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("check links: %w", err)
	}

	return Result{
		URL:         pg.URL.String(),
		HTMLVersion: p.htmlVersion,
		Title:       p.title,
		Headings:    p.headings,
		Links: LinkStats{
			Internal:     len(l.internal),
			External:     len(l.external),
			Checked:      checked,
			Inaccessible: inaccessible,
		},
		HasLoginForm: hasLoginForm(p.forms),
	}, nil
}

// countChecked maps the per-URL results back to every link, skipping links over the checker's cap.
func countChecked(links []string, results []linkcheck.Result) (checked, inaccessible int) {
	accessible := make(map[string]bool, len(results))
	for _, r := range results {
		accessible[r.URL] = r.Accessible()
	}
	for _, link := range links {
		ok, found := accessible[link]
		if !found {
			continue
		}
		checked++
		if !ok {
			inaccessible++
		}
	}
	return checked, inaccessible
}
