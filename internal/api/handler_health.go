package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleHealth reports that the process is up and serving, it checks no dependencies, so a slow target site never fails it.
func (s *server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
