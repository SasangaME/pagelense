package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
)

// FetchError is returned when a page cannot be fetched. Message is user-safe; Err is the cause.
type FetchError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *FetchError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *FetchError) Unwrap() error { return e.Err }

// statusError reports an upstream non-2xx response.
func statusError(code int) *FetchError {
	msg := fmt.Sprintf("upstream returned %d", code)
	if text := http.StatusText(code); text != "" {
		msg += " " + text
	}
	return &FetchError{StatusCode: code, Message: msg}
}

// transportError maps a network error to a FetchError; context.Canceled passes through.
func transportError(host string, err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}

	var (
		netErr net.Error
		dnsErr *net.DNSError
	)
	switch {
	case errors.Is(err, ErrBlockedAddress):
		return &FetchError{http.StatusForbidden, fmt.Sprintf("%s resolves to a private or reserved address", host), err}
	case errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout():
		return &FetchError{http.StatusGatewayTimeout, fmt.Sprintf("timed out fetching %s", host), err}
	case errors.As(err, &dnsErr):
		return &FetchError{http.StatusBadGateway, fmt.Sprintf("could not resolve host %s", host), err}
	case errors.Is(err, syscall.ECONNREFUSED):
		return &FetchError{http.StatusBadGateway, fmt.Sprintf("connection refused by %s", host), err}
	case errors.Is(err, ErrTooManyRedirects):
		return &FetchError{http.StatusBadGateway, fmt.Sprintf("too many redirects fetching %s", host), err}
	case errors.Is(err, ErrBodyTooLarge):
		return &FetchError{http.StatusBadGateway, fmt.Sprintf("page at %s is too large", host), err}
	default:
		return &FetchError{http.StatusBadGateway, fmt.Sprintf("could not fetch %s", host), err}
	}
}
