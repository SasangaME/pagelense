// Command web serves the frontend pages and calls the backend API.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SasangaME/pagelense/internal/admin"
	"github.com/SasangaME/pagelense/internal/config"
	"github.com/SasangaME/pagelense/internal/frontend"
	"github.com/SasangaME/pagelense/internal/metrics"
	"github.com/SasangaME/pagelense/internal/serve"
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

	reg := metrics.NewRegistry()

	// the request logger covers what Gin's debug output would print
	gin.SetMode(gin.ReleaseMode)
	handler, err := frontend.New(metrics.InstrumentBackend(reg, client), log, metrics.NewHTTP(reg).Middleware())
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
	adminSrv := admin.NewServer(cfg.AdminPort, reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		// once shutdown starts, a second Ctrl+C gets Go's default handling and exits at once
		<-ctx.Done()
		stop()
	}()

	log.Info("using backend", "api_url", cfg.APIURL)
	return serve.Run(ctx, log, cfg.ShutdownTimeout,
		serve.Server{Name: "web", Server: srv},
		serve.Server{Name: "admin", Server: adminSrv},
	)
}
