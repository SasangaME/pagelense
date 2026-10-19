// Package linkcheck checks whether links are reachable.
package linkcheck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Result is the outcome of checking one link.
type Result struct {
	URL        string
	StatusCode int // 0 if there was no response
	Err        error
}

// Accessible reports whether the link answered below 400.
func (r Result) Accessible() bool {
	return r.Err == nil && r.StatusCode < 400
}

var errBudgetExceeded = errors.New("link check budget exceeded")

// Options controls how links are checked.
type Options struct {
	Workers   int
	Timeout   time.Duration // per link, 0 means none
	Budget    time.Duration // whole Check call, 0 means none
	MaxLinks  int           // 0 means no limit
	UserAgent string
}

// Checker checks links with a pool of workers.
type Checker struct {
	client    *http.Client
	workers   int
	timeout   time.Duration
	budget    time.Duration
	maxLinks  int
	userAgent string
}

// New returns a Checker that uses client.
func New(client *http.Client, opts Options) *Checker {
	return &Checker{
		client:    client,
		workers:   max(opts.Workers, 1),
		timeout:   opts.Timeout,
		budget:    opts.Budget,
		maxLinks:  opts.MaxLinks,
		userAgent: opts.UserAgent,
	}
}

// Check checks the unique urls and returns results in completion order.
func (c *Checker) Check(ctx context.Context, urls []string) []Result {
	urls = unique(urls, c.maxLinks)
	if c.budget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, c.budget, errBudgetExceeded)
		defer cancel()
	}
	jobs := make(chan string)
	results := make(chan Result)

	var wg sync.WaitGroup
	for range min(c.workers, len(urls)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				r := c.check(ctx, u)
				// cut off by the budget: not counted as checked
				if r.Err != nil && errors.Is(context.Cause(ctx), errBudgetExceeded) {
					continue
				}
				results <- r
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, u := range urls {
			// select picks at random when both are ready, so check ctx first
			if ctx.Err() != nil {
				return
			}
			select {
			case jobs <- u:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	out := make([]Result, 0, len(urls))
	for r := range results {
		out = append(out, r)
	}
	return out
}

// some servers answer HEAD with 404 or 405 but serve GET fine, so retry with GET
func (c *Checker) check(ctx context.Context, rawURL string) Result {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	status, err := c.request(ctx, http.MethodHead, rawURL)
	if err == nil && status >= http.StatusBadRequest {
		status, err = c.request(ctx, http.MethodGet, rawURL)
	}
	return Result{URL: rawURL, StatusCode: status, Err: err}
}

func (c *Checker) request(ctx context.Context, method, rawURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func unique(urls []string, limit int) []string {
	seen := make(map[string]bool, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if limit > 0 && len(out) == limit {
			break
		}
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}
