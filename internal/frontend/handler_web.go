package frontend

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/analyzer"
)

// pageData is what every page template receives; each page uses the fields it needs.
type pageData struct {
	URL    string           // submitted URL, kept in the form so the user can fix it
	Result *analyzer.Result // result page only
	Status int              // error page only
	Error  string           // error page only
}

// handleIndex renders the URL form.
func (s *Server) handleIndex(c *gin.Context) {
	c.HTML(http.StatusOK, "index", pageData{})
}

// handleAnalyze analyzes the submitted URL and renders the result, or the error page with the form still filled in.
func (s *Server) handleAnalyze(c *gin.Context) {
	rawURL := c.PostForm("url")
	res, err := s.analyzer.Analyze(c.Request.Context(), rawURL)
	if err != nil {
		status, msg := errorResponse(err)
		c.HTML(status, "error", pageData{URL: rawURL, Status: status, Error: msg})
		return
	}
	c.HTML(http.StatusOK, "result", pageData{URL: rawURL, Result: &res})
}
