package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleHealth reports that the server is up. Placeholder until handler_health.go is implemented.
func (s *Server) handleHealth(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}
