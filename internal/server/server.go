// Package server provides the HTTP routes, handlers and middleware.
package server

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/web"
)

// Analyzer analyzes a web page. *analyzer.Analyzer implements it.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analyzer.Result, error)
}

// Server holds the dependencies shared by the handlers.
type Server struct {
	analyzer Analyzer
}

// New returns the app's HTTP handler: web pages, JSON API, health check and static files.
func New(a Analyzer) (http.Handler, error) {
	pages, err := parsePages(web.FS)
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(web.FS, "static")
	if err != nil {
		return nil, fmt.Errorf("static files: %w", err)
	}

	s := &Server{analyzer: a}
	r := gin.New()
	r.HTMLRender = pages
	r.HandleMethodNotAllowed = true
	// nil means X-Forwarded-For is ignored and ClientIP is the TCP peer
	if err := r.SetTrustedProxies(nil); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}

	r.GET("/", s.handleIndex)
	r.POST("/analyze", s.handleAnalyze)
	r.POST("/api/v1/analyze", s.handleAPIAnalyze)
	r.GET("/healthz", s.handleHealth)
	// a single file instead of StaticFS, which would list directories
	r.StaticFileFS("/static/style.css", "style.css", http.FS(static))

	return r, nil
}

// pageNames are the templates in web/templates that render inside layout.html.
var pageNames = []string{"index", "result", "error"}

// pages maps a page name to its template set; it implements render.HTMLRender for c.HTML.
type pages map[string]*template.Template

// Instance renders the "layout" template of the named page; each page defines the "content" block the layout includes.
func (p pages) Instance(name string, data any) render.Render {
	return render.HTML{Template: p[name], Name: "layout", Data: data}
}

// parsePages parses each page with its own copy of the layout, since every page defines "content".
func parsePages(fsys fs.FS) (pages, error) {
	p := make(pages, len(pageNames))
	for _, name := range pageNames {
		t, err := template.ParseFS(fsys, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", name, err)
		}
		p[name] = t
	}
	return p, nil
}
