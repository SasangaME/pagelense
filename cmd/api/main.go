// Command api serves the backend JSON API that analyzes web pages.
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
	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/api"
	"github.com/SasangaME/pagelense/internal/config"
	"github.com/SasangaME/pagelense/internal/fetcher"
	"github.com/SasangaME/pagelense/internal/linkcheck"
	"github.com/SasangaME/pagelense/internal/metrics"
	"github.com/SasangaME/pagelense/internal/serve"
)

const (
	maxRedirects = 10
	maxBodyBytes = 5 << 20
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

	reg := metrics.NewRegistry()
	a := metrics.InstrumentAnalyzer(reg, analyzer.New(f, metrics.InstrumentChecker(reg, checker)))

	gin.SetMode(gin.ReleaseMode)
	handler, err := api.New(a, log, metrics.NewHTTP(reg).Middleware())
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// no WriteTimeout: an analysis can take FETCH_TIMEOUT + LINK_BUDGET
	}
	adminSrv := admin.NewServer(cfg.AdminPort, reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		// a second Ctrl+C exits immediately
		<-ctx.Done()
		stop()
	}()

	return serve.Run(ctx, log, cfg.ShutdownTimeout,
		serve.Server{Name: "api", Server: srv},
		serve.Server{Name: "admin", Server: adminSrv},
	)
}
