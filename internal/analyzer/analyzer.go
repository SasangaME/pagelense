// Package analyzer fetches a web page and reports what it contains.
package analyzer

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/SasangaME/pagelense/internal/fetcher"
	"github.com/SasangaME/pagelense/internal/linkcheck"
)

// Fetcher downloads a page.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (*fetcher.Page, error)
}

// LinkChecker checks whether links are reachable.
type LinkChecker interface {
	Check(ctx context.Context, urls []string) []linkcheck.Result
}

// Analyzer analyzes web pages.
type Analyzer struct {
	fetcher Fetcher
	checker LinkChecker
}

// New returns an Analyzer.
func New(f Fetcher, c LinkChecker) *Analyzer {
	return &Analyzer{fetcher: f, checker: c}
}

// Analyze fetches rawURL, parses the page and checks its links.
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

	l := classifyLinks(pg.URL, p.baseHref, p.hrefs)
	all := slices.Concat(l.internal, l.external)

	checked, inaccessible := countChecked(all, a.checker.Check(ctx, all))
	// canceled checks look inaccessible, so don't report partial counts
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
		HasLoginForm: p.hasLogin,
	}, nil
}

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
