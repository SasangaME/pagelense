// Package linkcheck checks link reachability concurrently.
package linkcheck

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Result is the outcome of checking a single link.
type Result struct {
	URL        string
	StatusCode int   // 0 when no response was received
	Err        error // network failure, timeout or cancellation
}

// Accessible reports whether the link answered with a non-error status.
func (r Result) Accessible() bool {
	return r.Err == nil && r.StatusCode < 400
}

// Options controls how links are checked.
type Options struct {
	Workers  int           // number of concurrent checks; values below 1 mean 1
	Timeout  time.Duration // per link, covering the HEAD and any GET fallback; 0 means no limit
	MaxLinks int           // most unique URLs checked per call; values below 1 mean no limit
}

// Checker checks links with a fixed number of workers. It is safe for concurrent use.
type Checker struct {
	client   *http.Client
	workers  int
	timeout  time.Duration
	maxLinks int
}

// New returns a Checker that sends requests with client.
func New(client *http.Client, opts Options) *Checker {
	return &Checker{client: client, workers: max(opts.Workers, 1), timeout: opts.Timeout, maxLinks: opts.MaxLinks}
}

// Check checks the first MaxLinks unique urls concurrently; results are in completion order and omit URLs skipped after ctx is canceled.
func (c *Checker) Check(ctx context.Context, urls []string) []Result {
	urls = unique(urls, c.maxLinks)
	jobs := make(chan string)
	results := make(chan Result)

	var wg sync.WaitGroup
	for range min(c.workers, len(urls)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				results <- c.check(ctx, u)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, u := range urls {
			// select picks at random when both cases are ready, so check ctx first
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

	// results is closed once every worker has finished
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

// check sends a HEAD request to rawURL, retrying with GET if the server does not allow HEAD.
func (c *Checker) check(ctx context.Context, rawURL string) Result {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	status, err := c.request(ctx, http.MethodHead, rawURL)
	if err == nil && status == http.StatusMethodNotAllowed {
		status, err = c.request(ctx, http.MethodGet, rawURL)
	}
	return Result{URL: rawURL, StatusCode: status, Err: err}
}

// request sends a method request to rawURL and returns the status code without reading the body.
func (c *Checker) request(ctx context.Context, method, rawURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// unique returns urls without duplicates, keeping first occurrences in order, cut to limit if limit > 0.
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
