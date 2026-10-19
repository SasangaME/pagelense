package frontend

import (
	"context"
	"errors"
	"net/http"
)

// statusClientClosedRequest is nginx's code for a client that went away before the response, only logs and metrics see it.
const statusClientClosedRequest = 499

// errorResponse maps an error from the backend client to an HTTP status and a user-facing message, causes stay in the logs.
func errorResponse(err error) (int, string) {
	var apiErr *APIError
	switch {
	case errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status <= 599:
		// the backend's status and message are shown unchanged, so the page matches the JSON API
		return apiErr.Status, apiErr.Message
	case errors.Is(err, context.Canceled):
		// the browser went away; logging it as 502 would count it as a backend failure
		return statusClientClosedRequest, "request was canceled"
	default:
		return http.StatusBadGateway, "could not get a result from the analysis service"
	}
}
