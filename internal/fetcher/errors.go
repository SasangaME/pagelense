package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
)

// FetchError is returned when a page cannot be fetched. Message is safe to show users.
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

func statusError(host string, code int) *FetchError {
	msg := fmt.Sprintf("%s returned %d", host, code)
	if text := http.StatusText(code); text != "" {
		msg += " " + text
	}
	return &FetchError{StatusCode: code, Message: msg}
}

func contentTypeError(host, contentType string) *FetchError {
	return &FetchError{
		StatusCode: http.StatusUnsupportedMediaType,
		Message:    fmt.Sprintf("page at %s is not HTML (%s)", host, contentType),
		Err:        ErrNotHTML,
	}
}

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
