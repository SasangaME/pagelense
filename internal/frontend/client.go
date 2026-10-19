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

const analyzePath = "/api/v1/analyze"

// APIError is returned when the backend fails. Message is safe to show users.
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

// Client calls the backend's JSON API.
type Client struct {
	http     *http.Client
	endpoint string
}

// NewClient returns a client for the backend at baseURL.
func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("API URL: %w", err)
	}
	return &Client{
		http:     &http.Client{Timeout: timeout},
		endpoint: u.JoinPath(analyzePath).String(),
	}, nil
}

type analyzeRequest struct {
	URL string `json:"url"`
}

type errorBody struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Analyze asks the backend to analyze rawURL.
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

	data, err := io.ReadAll(resp.Body)
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

func statusError(status int, data []byte) *APIError {
	var eb errorBody
	if err := json.Unmarshal(data, &eb); err != nil || eb.Error.Message == "" {
		return &APIError{
			Status:  http.StatusBadGateway,
			Message: "analysis service sent an unexpected response",
			Err:     fmt.Errorf("backend returned %d with a body that is not an API error", status),
		}
	}
	return &APIError{Status: status, Message: eb.Error.Message}
}

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
