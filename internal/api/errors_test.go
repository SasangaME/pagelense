package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/fetcher"
)

// fetchErr wraps a FetchError the way the analyzer does.
func fetchErr(code int, msg string) error {
	return fmt.Errorf("fetch page: %w", &fetcher.FetchError{StatusCode: code, Message: msg, Err: errors.New("cause")})
}

func TestErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"invalid URL", fmt.Errorf("%w: must start with http:// or https://", analyzer.ErrInvalidURL),
			http.StatusBadRequest, "invalid URL: must start with http:// or https://"},
		{"upstream 404", fetchErr(http.StatusNotFound, "upstream returned 404 Not Found"),
			http.StatusNotFound, "upstream returned 404 Not Found"},
		{"upstream 503", fetchErr(http.StatusServiceUnavailable, "upstream returned 503 Service Unavailable"),
			http.StatusServiceUnavailable, "upstream returned 503 Service Unavailable"},
		{"blocked address", fetchErr(http.StatusForbidden, "localhost resolves to a private or reserved address"),
			http.StatusForbidden, "localhost resolves to a private or reserved address"},
		{"not HTML", fetchErr(http.StatusUnsupportedMediaType, "page at example.com is not HTML (image/png)"),
			http.StatusUnsupportedMediaType, "page at example.com is not HTML (image/png)"},
		{"upstream timeout", fetchErr(http.StatusGatewayTimeout, "timed out fetching example.com"),
			http.StatusGatewayTimeout, "timed out fetching example.com"},
		{"upstream 304", fetchErr(http.StatusNotModified, "upstream returned 304 Not Modified"),
			http.StatusBadGateway, "upstream returned 304 Not Modified"},
		{"canceled", fmt.Errorf("check links: %w", context.Canceled),
			statusClientClosedRequest, "Request was canceled."},
		{"deadline", fmt.Errorf("check links: %w", context.DeadlineExceeded),
			http.StatusGatewayTimeout, "Analysis took too long."},
		{"unexpected", errors.New("parse html: boom"),
			http.StatusInternalServerError, "Something went wrong while analyzing the page."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := errorResponse(tt.err)
			if status != tt.wantStatus || msg != tt.wantMsg {
				t.Errorf("got (%d, %q), want (%d, %q)", status, msg, tt.wantStatus, tt.wantMsg)
			}
		})
	}
}

func TestAnalyzeFetchErrorJSON(t *testing.T) {
	fake := &fakeAnalyzer{err: fetchErr(http.StatusNotFound, "upstream returned 404 Not Found")}
	w := postJSON(newAPIHandler(t, fake), `{"url": "https://example.com/missing"}`)

	if got := decodeError(t, w, http.StatusNotFound); got.Message != "upstream returned 404 Not Found" {
		t.Errorf("message = %q", got.Message)
	}
}
