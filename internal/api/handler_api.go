package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleAnalyze returns the analysis as JSON. Placeholder until handler_api.go is implemented.
func (s *Server) handleAnalyze(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}
