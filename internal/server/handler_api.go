package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleAPIAnalyze returns the analysis as JSON. Placeholder until handler_api.go is implemented.
func (s *Server) handleAPIAnalyze(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}
