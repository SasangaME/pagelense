// Package fetcher downloads web pages.
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"
)

var (
	// ErrTooManyRedirects is returned after Options.MaxRedirects redirects.
	ErrTooManyRedirects = errors.New("too many redirects")
	// ErrBodyTooLarge is returned for bodies over Options.MaxBodyBytes.
	ErrBodyTooLarge = errors.New("response body too large")
	// ErrNotHTML is returned for responses that are not HTML.
	ErrNotHTML = errors.New("response is not HTML")
)

// Options controls how pages are fetched.
type Options struct {
	Timeout      time.Duration
	MaxRedirects int // 0 means redirects are not followed
	MaxBodyBytes int64
	UserAgent    string
	// AllowPrivateIPs is for tests against local servers.
	AllowPrivateIPs bool
}

// Page is a downloaded HTML page.
type Page struct {
	URL  *url.URL // after redirects
	Body []byte
}

// Fetcher downloads web pages.
type Fetcher struct {
	client       *http.Client
	maxBodyBytes int64
	userAgent    string
}

// New returns a Fetcher.
func New(opts Options) *Fetcher {
	maxRedirects := opts.MaxRedirects
	return &Fetcher{
		client: &http.Client{
			Timeout:   opts.Timeout,
			Transport: newTransport(opts.AllowPrivateIPs),
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) > maxRedirects {
					return fmt.Errorf("%w (limit %d)", ErrTooManyRedirects, maxRedirects)
				}
				return nil
			},
		},
		maxBodyBytes: opts.MaxBodyBytes,
		userAgent:    opts.UserAgent,
	}
}

// Client returns the HTTP client, so link checks can share its dialer and pool.
func (f *Fetcher) Client() *http.Client {
	return f.client
}

// Fetch downloads rawURL. Failures are returned as *FetchError.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, transportError(req.URL.Host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, statusError(req.URL.Host, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !isHTML(contentType) {
		return nil, contentTypeError(req.URL.Host, contentType)
	}

	// +1 to detect bodies over the limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodyBytes+1))
	if err != nil {
		return nil, transportError(req.URL.Host, fmt.Errorf("read body: %w", err))
	}
	if int64(len(body)) > f.maxBodyBytes {
		return nil, transportError(req.URL.Host, fmt.Errorf("%w (limit %d bytes)", ErrBodyTooLarge, f.maxBodyBytes))
	}

	return &Page{URL: resp.Request.URL, Body: body}, nil
}

func isHTML(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}
