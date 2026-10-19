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
		{"page 404", fetchErr(http.StatusNotFound, "example.com returned 404 Not Found"),
			http.StatusNotFound, "example.com returned 404 Not Found"},
		{"page 503", fetchErr(http.StatusServiceUnavailable, "example.com returned 503 Service Unavailable"),
			http.StatusServiceUnavailable, "example.com returned 503 Service Unavailable"},
		{"blocked address", fetchErr(http.StatusForbidden, "localhost resolves to a private or reserved address"),
			http.StatusForbidden, "localhost resolves to a private or reserved address"},
		{"not HTML", fetchErr(http.StatusUnsupportedMediaType, "page at example.com is not HTML (image/png)"),
			http.StatusUnsupportedMediaType, "page at example.com is not HTML (image/png)"},
		{"upstream timeout", fetchErr(http.StatusGatewayTimeout, "timed out fetching example.com"),
			http.StatusGatewayTimeout, "timed out fetching example.com"},
		{"page 304", fetchErr(http.StatusNotModified, "example.com returned 304 Not Modified"),
			http.StatusBadGateway, "example.com returned 304 Not Modified"},
		{"canceled", fmt.Errorf("check links: %w", context.Canceled),
			statusClientClosedRequest, "request was canceled"},
		{"deadline", fmt.Errorf("check links: %w", context.DeadlineExceeded),
			http.StatusGatewayTimeout, "analysis took too long"},
		{"unexpected", errors.New("parse html: boom"),
			http.StatusInternalServerError, msgInternal},
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
	fake := &fakeAnalyzer{err: fetchErr(http.StatusNotFound, "example.com returned 404 Not Found")}
	w := postJSON(newAPIHandler(t, fake), `{"url": "https://example.com/missing"}`)

	if got := decodeError(t, w, http.StatusNotFound); got.Message != "example.com returned 404 Not Found" {
		t.Errorf("message = %q", got.Message)
	}
}
