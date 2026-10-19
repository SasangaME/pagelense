package api

import (
	"errors"
	"net/http"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/fetcher"
)

const msgInternal = "something went wrong while analyzing the page"

func errorResponse(err error) (int, string) {
	var fe *fetcher.FetchError
	switch {
	case errors.Is(err, analyzer.ErrInvalidURL):
		return http.StatusBadRequest, err.Error()
	case errors.As(err, &fe):
		return fe.StatusCode, fe.Message
	default:
		return http.StatusInternalServerError, msgInternal
	}
}
