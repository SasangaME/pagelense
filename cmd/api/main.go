// Command api serves the backend JSON API that analyzes web pages.
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/api"
	"github.com/SasangaME/pagelense/internal/config"
	"github.com/SasangaME/pagelense/internal/fetcher"
	"github.com/SasangaME/pagelense/internal/linkcheck"
)

// Fetch limits that are not worth an env var.
const (
	maxRedirects = 10
	maxBodyBytes = 5 << 20 // 5 MiB
	userAgent    = "pagelense/1.0 (+https://github.com/SasangaME/pagelense)"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	f := fetcher.New(fetcher.Options{
		Timeout:      cfg.FetchTimeout,
		MaxRedirects: maxRedirects,
		MaxBodyBytes: maxBodyBytes,
		UserAgent:    userAgent,
	})
	checker := linkcheck.New(f.Client(), linkcheck.Options{
		Workers:   cfg.LinkWorkers,
		Timeout:   cfg.LinkTimeout,
		Budget:    cfg.LinkBudget,
		MaxLinks:  cfg.MaxLinks,
		UserAgent: userAgent,
	})

	// the request logger covers what Gin's debug output would print
	gin.SetMode(gin.ReleaseMode)
	handler, err := api.New(analyzer.New(f, checker), log)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// no WriteTimeout: an analysis can legitimately take FETCH_TIMEOUT plus LINK_BUDGET
	}
	log.Info("api listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("api server: %w", err)
	}
	return nil
}
