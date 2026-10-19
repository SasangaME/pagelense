package frontend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/middleware"
)

// analyzePath is the backend endpoint, relative to API_URL.
const analyzePath = "/api/v1/analyze"

// maxResponseBytes caps a backend response; a result or error body is a few hundred bytes.
const maxResponseBytes = 1 << 20

// APIError is returned when the backend answers with an error or cannot be reached. Message is user-safe; Err is the cause.
type APIError struct {
	Status  int
	Message string
	Err     error
}

func (e *APIError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *APIError) Unwrap() error { return e.Err }

// Client calls the backend's JSON API. It implements Analyzer.
type Client struct {
	http     *http.Client
	endpoint string
}

// NewClient returns a client for the backend at baseURL; timeout bounds each call, so it must outlast the backend's fetch plus link checks.
func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("API URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("API URL %q: must be an absolute http:// or https:// URL", baseURL)
	}
	return &Client{
		http:     &http.Client{Timeout: timeout},
		endpoint: u.JoinPath(analyzePath).String(),
	}, nil
}

type analyzeRequest struct {
	URL string `json:"url"`
}

// errorBody matches the backend's error JSON; internal/api's contract test keeps the field names fixed.
type errorBody struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Analyze asks the backend to analyze rawURL; backend errors keep their status, unreachable gives 502, no answer in time 504.
func (c *Client) Analyze(ctx context.Context, rawURL string) (analyzer.Result, error) {
	body, err := json.Marshal(analyzeRequest{URL: rawURL})
	if err != nil {
		return analyzer.Result{}, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return analyzer.Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if id := middleware.RequestIDFrom(ctx); id != "" {
		req.Header.Set(middleware.HeaderRequestID, id)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return analyzer.Result{}, transportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return analyzer.Result{}, transportError(err)
	}

	if resp.StatusCode != http.StatusOK {
		return analyzer.Result{}, statusError(resp.StatusCode, data)
	}
	var res analyzer.Result
	if err := json.Unmarshal(data, &res); err != nil {
		return analyzer.Result{}, &APIError{http.StatusBadGateway, "analysis service sent an invalid response", err}
	}
	return res, nil
}

// statusError turns a non-200 backend response into an APIError; a body that is not the API's error JSON gives 502.
func statusError(status int, data []byte) *APIError {
	var eb errorBody
	if err := json.Unmarshal(data, &eb); err != nil || eb.Error.Message == "" || status < 400 || status > 599 {
		return &APIError{
			Status:  http.StatusBadGateway,
			Message: "analysis service sent an unexpected response",
			Err:     fmt.Errorf("backend returned %d with a body that is not an API error", status),
		}
	}
	return &APIError{Status: status, Message: eb.Error.Message}
}

// transportError maps a failed call to an APIError; context.Canceled passes through, like in the fetcher.
func transportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		return &APIError{http.StatusGatewayTimeout, "analysis service took too long to respond", err}
	}
	return &APIError{http.StatusBadGateway, "analysis service is unavailable, try again later", err}
}
