package api

import "net/http"

// errorResponse maps an analysis error to an HTTP status and a user-facing message. Placeholder until errors.go is implemented.
func errorResponse(_ error) (int, string) {
	return http.StatusInternalServerError, "Something went wrong while analyzing the page."
}
