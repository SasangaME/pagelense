package frontend

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleHealth reports that the process is up and serving; it does not call the backend, so a backend outage never fails it.
func (s *server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
