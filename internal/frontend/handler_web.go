package frontend

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/analyzer"
)

type pageData struct {
	URL    string
	Result *analyzer.Result
	Status int
	Error  string
}

func (s *server) handleIndex(c *gin.Context) {
	s.render(c, http.StatusOK, "index", pageData{})
}

func (s *server) handleAnalyze(c *gin.Context) {
	rawURL := c.PostForm("url")
	res, err := s.analyzer.Analyze(c.Request.Context(), rawURL)
	if err != nil {
		_ = c.Error(err)
		status, msg := errorResponse(err)
		s.render(c, status, "error", pageData{URL: rawURL, Status: status, Error: msg})
		return
	}
	s.render(c, http.StatusOK, "result", pageData{URL: rawURL, Result: &res})
}
