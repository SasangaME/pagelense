package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/middleware"
)

var _ Analyzer = (*Client)(nil)

func newTestClient(t *testing.T, baseURL string, timeout time.Duration) *Client {
	t.Helper()
	c, err := NewClient(baseURL, timeout)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

type fakeBackend struct {
	status int
	body   string

	mu  sync.Mutex
	got recordedRequest
}

type recordedRequest struct {
	method, path, contentType, requestID, body string
}

func (b *fakeBackend) last() recordedRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.got
}

func (b *fakeBackend) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		b.got = recordedRequest{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			requestID:   r.Header.Get(middleware.HeaderRequestID),
			body:        string(data),
		}
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(b.status)
		_, _ = io.WriteString(w, b.body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClientSuccess(t *testing.T) {
	want := analyzer.Result{
		URL:          "https://example.com/home",
		HTMLVersion:  "HTML5",
		Title:        "Example",
		Headings:     analyzer.Headings{H1: 1, H3: 2},
		Links:        analyzer.LinkStats{Internal: 4, External: 1, Checked: 5, Inaccessible: 1},
		HasLoginForm: true,
	}
	body, _ := json.Marshal(want)
	b := &fakeBackend{status: http.StatusOK, body: string(body)}
	srv := b.start(t)

	ctx := middleware.WithRequestID(context.Background(), "req-123")
	got, err := newTestClient(t, srv.URL, time.Second).Analyze(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got != want {
		t.Errorf("result = %+v, want %+v", got, want)
	}

	req := b.last()
	if req.method != http.MethodPost || req.path != "/api/v1/analyze" {
		t.Errorf("request = %s %s, want POST /api/v1/analyze", req.method, req.path)
	}
	if req.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", req.contentType)
	}
	if req.body != `{"url":"https://example.com"}` {
		t.Errorf("request body = %s", req.body)
	}
	if req.requestID != "req-123" {
		t.Errorf("X-Request-ID = %q, want req-123", req.requestID)
	}
}

func TestClientNoRequestID(t *testing.T) {
	b := &fakeBackend{status: http.StatusOK, body: `{}`}
	srv := b.start(t)

	if _, err := newTestClient(t, srv.URL, time.Second).Analyze(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if id := b.last().requestID; id != "" {
		t.Errorf("X-Request-ID = %q, want none", id)
	}
}

func TestClientBaseURLWithPath(t *testing.T) {
	b := &fakeBackend{status: http.StatusOK, body: `{}`}
	srv := b.start(t)

	if _, err := newTestClient(t, srv.URL+"/backend/", time.Second).Analyze(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if path := b.last().path; path != "/backend/api/v1/analyze" {
		t.Errorf("path = %q, want /backend/api/v1/analyze", path)
	}
}

func TestClientErrors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "invalid URL",
			status:      http.StatusBadRequest,
			body:        `{"error":{"status":400,"message":"invalid URL: must start with http:// or https://"}}`,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "invalid URL: must start with http:// or https://",
		},
		{
			name:        "page 404",
			status:      http.StatusNotFound,
			body:        `{"error":{"status":404,"message":"example.com returned 404 Not Found"}}`,
			wantStatus:  http.StatusNotFound,
			wantMessage: "example.com returned 404 Not Found",
		},
		{
			name:        "upstream timeout",
			status:      http.StatusGatewayTimeout,
			body:        `{"error":{"status":504,"message":"timed out fetching example.com"}}`,
			wantStatus:  http.StatusGatewayTimeout,
			wantMessage: "timed out fetching example.com",
		},
		{
			name:        "error without JSON body",
			status:      http.StatusServiceUnavailable,
			body:        `<html>Service Unavailable</html>`,
			wantStatus:  http.StatusBadGateway,
			wantMessage: "analysis service sent an unexpected response",
		},
		{
			name:        "error JSON without message",
			status:      http.StatusInternalServerError,
			body:        `{"error":{"status":500}}`,
			wantStatus:  http.StatusBadGateway,
			wantMessage: "analysis service sent an unexpected response",
		},
		{
			name:        "malformed result",
			status:      http.StatusOK,
			body:        `{"url":`,
			wantStatus:  http.StatusBadGateway,
			wantMessage: "analysis service sent an invalid response",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := (&fakeBackend{status: tt.status, body: tt.body}).start(t)

			_, err := newTestClient(t, srv.URL, time.Second).Analyze(context.Background(), "https://example.com")
			assertAPIError(t, err, tt.wantStatus, tt.wantMessage)
		})
	}
}

func newHangingBackend(t *testing.T, onRequest func()) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if onRequest != nil {
			onRequest()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // runs before srv.Close
	return srv
}

func TestClientUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()

	_, err := newTestClient(t, srv.URL, time.Second).Analyze(context.Background(), "https://example.com")
	assertAPIError(t, err, http.StatusBadGateway, "analysis service is unavailable, try again later")
}

func TestClientTimeout(t *testing.T) {
	srv := newHangingBackend(t, nil)

	_, err := newTestClient(t, srv.URL, 50*time.Millisecond).Analyze(context.Background(), "https://example.com")
	assertAPIError(t, err, http.StatusGatewayTimeout, "analysis service took too long to respond")
}

func TestClientContextDeadline(t *testing.T) {
	srv := newHangingBackend(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := newTestClient(t, srv.URL, time.Second).Analyze(ctx, "https://example.com")
	assertAPIError(t, err, http.StatusGatewayTimeout, "analysis service took too long to respond")
}

func TestClientCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := newHangingBackend(t, cancel)

	_, err := newTestClient(t, srv.URL, time.Second).Analyze(ctx, "https://example.com")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Errorf("err = %v, want plain context.Canceled, not an APIError", err)
	}
}

func TestNewClientInvalidURL(t *testing.T) {
	for _, raw := range []string{"http://api:8081/%zz"} {
		if _, err := NewClient(raw, time.Second); err == nil {
			t.Errorf("NewClient(%q) = nil error, want error", raw)
		}
	}
}

func TestAPIErrorMessage(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	err := &APIError{Status: http.StatusBadGateway, Message: "unavailable", Err: cause}
	if got := err.Error(); got != "unavailable: dial tcp: connection refused" {
		t.Errorf("Error() = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is does not find the cause")
	}
	if got := (&APIError{Message: "bad input"}).Error(); got != "bad input" {
		t.Errorf("Error() without cause = %q", got)
	}
}

func assertAPIError(t *testing.T, err error, wantStatus int, wantMessage string) {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v (%T), want *APIError", err, err)
	}
	if apiErr.Status != wantStatus {
		t.Errorf("Status = %d, want %d", apiErr.Status, wantStatus)
	}
	if apiErr.Message != wantMessage {
		t.Errorf("Message = %q, want %q", apiErr.Message, wantMessage)
	}
	if strings.Contains(apiErr.Message, "127.0.0.1") {
		t.Errorf("Message %q leaks the backend address", apiErr.Message)
	}
}
