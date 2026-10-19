// Package frontend serves the web pages and calls the backend API.
package frontend

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/middleware"
	"github.com/SasangaME/pagelense/web"
)

// Analyzer analyzes a web page.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analyzer.Result, error)
}

type server struct {
	analyzer Analyzer
	pages    pages
}

// partialHeader is set by app.js, which only needs the page content.
const partialHeader = "X-Partial"

func (s *server) render(c *gin.Context, status int, name string, data pageData) {
	block := "layout"
	if c.GetHeader(partialHeader) != "" {
		block = "content"
	}
	c.Header("Vary", partialHeader)
	c.Render(status, render.HTML{Template: s.pages[name], Name: block, Data: data})
}

// New returns the frontend's HTTP handler; extra middleware runs first.
func New(a Analyzer, log *slog.Logger, extra ...gin.HandlerFunc) (http.Handler, error) {
	return newHandler(a, web.FS, log, extra...)
}

func newHandler(a Analyzer, fsys fs.FS, log *slog.Logger, extra ...gin.HandlerFunc) (http.Handler, error) {
	pages, err := parsePages(fsys)
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(fsys, "static")
	if err != nil {
		return nil, fmt.Errorf("static files: %w", err)
	}

	s := &server{analyzer: a, pages: pages}
	r := gin.New()
	r.HandleMethodNotAllowed = true
	if err := r.SetTrustedProxies(nil); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}
	r.Use(extra...)
	r.Use(
		middleware.RequestID(),
		middleware.Logger(log),
		middleware.Recovery(log, func(c *gin.Context) {
			s.render(c, http.StatusInternalServerError, "error", pageData{
				URL:    c.PostForm("url"),
				Status: http.StatusInternalServerError,
				Error:  "something went wrong, try again",
			})
		}),
	)

	r.GET("/", s.handleIndex)
	r.POST("/analyze", s.handleAnalyze)
	r.GET("/healthz", s.handleHealth)
	r.StaticFileFS("/static/style.css", "style.css", http.FS(static))
	r.StaticFileFS("/static/app.js", "app.js", http.FS(static))

	return r, nil
}

var pageNames = []string{"index", "result", "error"}

type pages map[string]*template.Template

var funcs = template.FuncMap{"statusText": http.StatusText}

// every page defines "content", so each needs its own copy of the layout
func parsePages(fsys fs.FS) (pages, error) {
	p := make(pages, len(pageNames))
	for _, name := range pageNames {
		t, err := template.New(name).Funcs(funcs).ParseFS(fsys, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", name, err)
		}
		p[name] = t
	}
	return p, nil
}
