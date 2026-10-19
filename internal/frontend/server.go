// Package frontend serves the web pages and gets each analysis from the backend API.
package frontend

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

// Analyzer analyzes a web page. The backend API client implements it; tests use a fake.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analyzer.Result, error)
}

// Server holds the dependencies shared by the handlers.
type Server struct {
	analyzer Analyzer
}

// New returns the frontend's HTTP handler: web pages, health check and static files.
func New(a Analyzer) (http.Handler, error) {
	return newHandler(a, web.FS)
}

// newHandler builds the handler from the templates and static files in fsys, so tests can use their own templates.
func newHandler(a Analyzer, fsys fs.FS) (http.Handler, error) {
	pages, err := parsePages(fsys)
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(fsys, "static")
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
