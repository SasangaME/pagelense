// Package frontend serves the web pages and gets each analysis from the backend API.
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

// Analyzer analyzes a web page. The backend API client implements it; tests use a fake.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analyzer.Result, error)
}

// server holds the dependencies shared by the handlers.
type server struct {
	analyzer Analyzer
	pages    pages
}

// partialHeader marks a request from app.js, which wants only the page content to swap into <main>.
const partialHeader = "X-Partial"

// render writes the named page: the whole layout, or only its "content" block for a partial request.
func (s *server) render(c *gin.Context, status int, name string, data pageData) {
	block := "layout"
	if c.GetHeader(partialHeader) != "" {
		block = "content"
	}
	// the same URL returns either form, so caches must key on the header
	c.Header("Vary", partialHeader)
	c.Render(status, render.HTML{Template: s.pages[name], Name: block, Data: data})
}

// New returns the frontend's HTTP handler with request IDs, logging and recovery; extra middleware, such as metrics, runs first.
func New(a Analyzer, log *slog.Logger, extra ...gin.HandlerFunc) (http.Handler, error) {
	return newHandler(a, web.FS, log, extra...)
}

// newHandler builds the handler from the templates and static files in fsys, so tests can use their own templates.
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
	// nil means X-Forwarded-For is ignored and ClientIP is the TCP peer
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
	// single files instead of StaticFS, which would list directories
	r.StaticFileFS("/static/style.css", "style.css", http.FS(static))
	r.StaticFileFS("/static/app.js", "app.js", http.FS(static))

	return r, nil
}

// pageNames are the templates in web/templates that render inside layout.html.
var pageNames = []string{"index", "result", "error"}

// pages maps a page name to its template set; render picks the "layout" or "content" block from it.
type pages map[string]*template.Template

// funcs are the helpers templates can call; statusText turns 404 into "Not Found".
var funcs = template.FuncMap{"statusText": http.StatusText}

// parsePages parses each page with its own copy of the layout, since every page defines "content".
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
