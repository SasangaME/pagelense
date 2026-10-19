package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/fetcher"
)

// statusClientClosedRequest is nginx's code for a client that went away before the response; only logs and metrics see it.
const statusClientClosedRequest = 499

// msgInternal is the message for a failure the user can do nothing about, the cause goes to the logs only.
const msgInternal = "something went wrong while analyzing the page"

// errorResponse maps an analysis error to an HTTP status and a user-facing message; causes stay in the logs.
func errorResponse(err error) (int, string) {
	var fe *fetcher.FetchError
	switch {
	case errors.Is(err, analyzer.ErrInvalidURL):
		// the message describes the user's own input, e.g. "invalid URL: must start with http:// or https://"
		return http.StatusBadRequest, err.Error()
	case errors.As(err, &fe):
		return fetchStatus(fe.StatusCode), fe.Message
	case errors.Is(err, context.Canceled):
		return statusClientClosedRequest, "request was canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "analysis took too long"
	default:
		return http.StatusInternalServerError, msgInternal
	}
}

// fetchStatus passes on an upstream 4xx/5xx status; any other code (e.g. 304) becomes 502, and the message still names it.
func fetchStatus(code int) int {
	if code >= 400 && code <= 599 {
		return code
	}
	return http.StatusBadGateway
}
