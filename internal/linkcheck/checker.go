// Package linkcheck checks link reachability concurrently.
package linkcheck

import (
	"context"
	"fmt"
	"net/http"
	"sync"
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

// Checker checks links with a fixed number of workers. It is safe for concurrent use.
type Checker struct {
	client  *http.Client
	workers int
}

// New returns a Checker that sends requests with client using up to workers goroutines.
func New(client *http.Client, workers int) *Checker {
	return &Checker{client: client, workers: max(workers, 1)}
}

// Check checks urls concurrently and returns results in completion order, skipping unsent URLs if ctx is canceled.
func (c *Checker) Check(ctx context.Context, urls []string) []Result {
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
