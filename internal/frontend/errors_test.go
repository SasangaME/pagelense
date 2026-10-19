package frontend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"invalid URL", &APIError{Status: http.StatusBadRequest, Message: "invalid URL: must start with http:// or https://"},
			http.StatusBadRequest, "invalid URL: must start with http:// or https://"},
		{"page 404", &APIError{Status: http.StatusNotFound, Message: "example.com returned 404 Not Found"},
			http.StatusNotFound, "example.com returned 404 Not Found"},
		{"backend unreachable", &APIError{Status: http.StatusBadGateway, Message: "analysis service is unavailable, try again later", Err: errors.New("connection refused")},
			http.StatusBadGateway, "analysis service is unavailable, try again later"},
		{"backend timeout", &APIError{Status: http.StatusGatewayTimeout, Message: "analysis service took too long to respond"},
			http.StatusGatewayTimeout, "analysis service took too long to respond"},
		{"wrapped API error", fmt.Errorf("analyze: %w", &APIError{Status: http.StatusForbidden, Message: "localhost resolves to a private or reserved address"}),
			http.StatusForbidden, "localhost resolves to a private or reserved address"},
		{"API error with non-error status", &APIError{Status: http.StatusOK, Message: "ok"},
			http.StatusBadGateway, "could not get a result from the analysis service"},
		{"canceled", context.Canceled,
			statusClientClosedRequest, "request was canceled"},
		{"unexpected", errors.New("encode request: boom"),
			http.StatusBadGateway, "could not get a result from the analysis service"},
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

func TestAnalyzeAPIErrorPage(t *testing.T) {
	fake := &fakeAnalyzer{err: &APIError{Status: http.StatusNotFound, Message: "example.com returned 404 Not Found"}}
	w := postForm(newWebHandler(t, fake), url.Values{"url": {"https://example.com/missing"}})

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	want := "url=https://example.com/missing|error|404|example.com returned 404 Not Found"
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestAnalyzeCauseIsLoggedNotShown(t *testing.T) {
	fake := &fakeAnalyzer{err: &APIError{
		Status:  http.StatusBadGateway,
		Message: "analysis service is unavailable, try again later",
		Err:     errors.New("dial tcp 10.0.0.7:8081: connect: connection refused"),
	}}
	var logs bytes.Buffer
	h, err := newHandler(fake, testPages, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}

	w := postForm(h, url.Values{"url": {"https://example.com"}})

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if strings.Contains(w.Body.String(), "10.0.0.7") {
		t.Errorf("page leaks the internal cause: %s", w.Body)
	}
	if !strings.Contains(logs.String(), "10.0.0.7") {
		t.Errorf("log is missing the cause: %s", logs.String())
	}
}
