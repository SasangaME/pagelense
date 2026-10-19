// Package api serves the backend JSON API that analyzes web pages.
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/analyzer"
)

// Analyzer analyzes a web page. *analyzer.Analyzer implements it.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analyzer.Result, error)
}

// Server holds the dependencies shared by the handlers.
type Server struct {
	analyzer Analyzer
}

// New returns the backend's HTTP handler: the analyze endpoint and health check.
func New(a Analyzer) (http.Handler, error) {
	s := &Server{analyzer: a}
	r := gin.New()
	r.HandleMethodNotAllowed = true
	// nil means X-Forwarded-For is ignored and ClientIP is the TCP peer
	if err := r.SetTrustedProxies(nil); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}

	r.POST("/api/v1/analyze", s.handleAnalyze)
	r.GET("/healthz", s.handleHealth)

	return r, nil
}
