// Package fetcher downloads web pages with timeouts, size limits and an SSRF-safe dialer.
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
	// ErrTooManyRedirects is returned when a page redirects more than Options.MaxRedirects times.
	ErrTooManyRedirects = errors.New("too many redirects")
	// ErrBodyTooLarge is returned when a response body is larger than Options.MaxBodyBytes.
	ErrBodyTooLarge = errors.New("response body too large")
	// ErrNotHTML is returned when the response is not an HTML document.
	ErrNotHTML = errors.New("response is not HTML")
)

// Options controls how pages are fetched.
type Options struct {
	Timeout      time.Duration // whole request, including redirects and reading the body
	MaxRedirects int           // 0 means redirects are not followed
	MaxBodyBytes int64
	UserAgent    string
	// AllowPrivateIPs disables the SSRF check; only for tests against local servers.
	AllowPrivateIPs bool
}

// Page is a downloaded web page.
type Page struct {
	URL         *url.URL // final URL after redirects; relative links resolve against it
	StatusCode  int
	ContentType string
	Body        []byte
}

// Fetcher downloads web pages. It is safe for concurrent use.
type Fetcher struct {
	client       *http.Client
	maxBodyBytes int64
	userAgent    string
}

// New returns a Fetcher configured by opts.
func New(opts Options) *Fetcher {
	maxRedirects := opts.MaxRedirects
	return &Fetcher{
		client: &http.Client{
			Timeout:   opts.Timeout,
			Transport: newTransport(opts.AllowPrivateIPs),
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				// len(via) == number of redirects including this one
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

// Fetch downloads rawURL. HTTP and network failures are returned as *FetchError.
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
		return nil, statusError(resp.StatusCode)
	}

	// checked before reading so non-HTML bodies are not downloaded
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

	if contentType == "" {
		contentType = http.DetectContentType(body)
		if !isHTML(contentType) {
			return nil, contentTypeError(req.URL.Host, contentType)
		}
	}

	return &Page{
		URL:         resp.Request.URL,
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Body:        body,
	}, nil
}

// isHTML reports whether contentType is text/html or application/xhtml+xml.
func isHTML(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}
