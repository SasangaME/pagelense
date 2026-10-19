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
	s.render(c, http.StatusOK, "index", pageData{})
}

// handleAnalyze analyzes the submitted URL and renders the result, or the error page with the form still filled in.
func (s *Server) handleAnalyze(c *gin.Context) {
	rawURL := c.PostForm("url")
	res, err := s.analyzer.Analyze(c.Request.Context(), rawURL)
	if err != nil {
		_ = c.Error(err) // for the logging middleware; the page only shows msg
		status, msg := errorResponse(err)
		s.render(c, status, "error", pageData{URL: rawURL, Status: status, Error: msg})
		return
	}
	s.render(c, http.StatusOK, "result", pageData{URL: rawURL, Result: &res})
}
