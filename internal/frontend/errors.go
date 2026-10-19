package frontend

import "net/http"

// errorResponse maps an error from the backend client to an HTTP status and a user-facing message. Placeholder until errors.go is implemented.
func errorResponse(_ error) (int, string) {
	return http.StatusInternalServerError, "Something went wrong while analyzing the page."
}
