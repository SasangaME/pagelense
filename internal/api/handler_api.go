package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxRequestBytes = 64 << 10

type analyzeRequest struct {
	URL string `json:"url"`
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (s *server) handleAnalyze(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)

	var req analyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(c, err, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		writeError(c, err, http.StatusBadRequest, `request body must be JSON like {"url": "https://example.com"}`)
		return
	}

	res, err := s.analyzer.Analyze(c.Request.Context(), req.URL)
	if err != nil {
		status, msg := errorResponse(err)
		writeError(c, err, status, msg)
		return
	}
	c.JSON(http.StatusOK, res)
}

// writeError also records err so the request log shows the cause.
func writeError(c *gin.Context, err error, status int, msg string) {
	_ = c.Error(err)
	writeErrorBody(c, status, msg)
}

func writeErrorBody(c *gin.Context, status int, msg string) {
	c.JSON(status, errorBody{Error: errorDetail{Status: status, Message: msg}})
}
