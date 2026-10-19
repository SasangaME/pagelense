// Package api serves the backend JSON API that analyzes web pages.
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/middleware"
)

// Analyzer analyzes a web page. *analyzer.Analyzer implements it.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analyzer.Result, error)
}

// server holds the dependencies shared by the handlers.
type server struct {
	analyzer Analyzer
}

// New returns the backend's HTTP handler with request IDs, logging and recovery; extra middleware, such as metrics, runs first.
func New(a Analyzer, log *slog.Logger, extra ...gin.HandlerFunc) (http.Handler, error) {
	s := &server{analyzer: a}
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
			writeErrorBody(c, http.StatusInternalServerError, msgInternal)
		}),
	)

	r.POST("/api/v1/analyze", s.handleAnalyze)
	r.GET("/healthz", s.handleHealth)

	return r, nil
}
