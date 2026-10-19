// Command web serves the frontend pages and calls the backend API.
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/config"
	"github.com/SasangaME/pagelense/internal/frontend"
)

func main() {
	if err := run(); err != nil {
		slog.Error("web stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadWeb(os.Getenv)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	client, err := frontend.NewClient(cfg.APIURL, cfg.APITimeout)
	if err != nil {
		return err
	}

	// the request logger covers what Gin's debug output would print
	gin.SetMode(gin.ReleaseMode)
	handler, err := frontend.New(client, log)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// no WriteTimeout: a result page waits up to API_TIMEOUT for the backend
	}
	log.Info("web listening", "addr", srv.Addr, "api_url", cfg.APIURL)
	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("web server: %w", err)
	}
	return nil
}
