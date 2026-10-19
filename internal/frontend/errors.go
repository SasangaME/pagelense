package frontend

import (
	"context"
	"errors"
	"net/http"
)

// nginx uses 499 for a client that closed the connection
const statusClientClosedRequest = 499

func errorResponse(err error) (int, string) {
	var apiErr *APIError
	switch {
	case errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status <= 599:
		return apiErr.Status, apiErr.Message
	case errors.Is(err, context.Canceled):
		return statusClientClosedRequest, "request was canceled"
	default:
		return http.StatusBadGateway, "could not get a result from the analysis service"
	}
}
