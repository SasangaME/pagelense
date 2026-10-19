package frontend

import (
	"errors"
	"net/http"
)

func errorResponse(err error) (int, string) {
	var apiErr *APIError
	switch {
	case errors.As(err, &apiErr):
		return apiErr.Status, apiErr.Message
	default:
		return http.StatusBadGateway, "could not get a result from the analysis service"
	}
}
