package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/fetcher"
)

// nginx uses 499 for a client that closed the connection
const statusClientClosedRequest = 499

const msgInternal = "something went wrong while analyzing the page"

func errorResponse(err error) (int, string) {
	var fe *fetcher.FetchError
	switch {
	case errors.Is(err, analyzer.ErrInvalidURL):
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

func fetchStatus(code int) int {
	if code >= 400 && code <= 599 {
		return code
	}
	return http.StatusBadGateway
}
