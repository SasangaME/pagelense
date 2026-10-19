package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleIndex renders the URL form. Placeholder until handler_web.go is implemented.
func (s *Server) handleIndex(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

// handleAnalyze renders the result page. Placeholder until handler_web.go is implemented.
func (s *Server) handleAnalyze(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}
