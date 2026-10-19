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

	"github.com/SasangaME/pagelense/internal/admin"
	"github.com/SasangaME/pagelense/internal/config"
	"github.com/SasangaME/pagelense/internal/frontend"
	"github.com/SasangaME/pagelense/internal/metrics"
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

	// either server failing stops the service; graceful shutdown comes with the signal handling
	errc := make(chan error, 2)
	go func() {
		log.Info("admin listening", "addr", adminSrv.Addr)
		errc <- fmt.Errorf("admin server: %w", adminSrv.ListenAndServe())
	}()
	go func() {
		log.Info("web listening", "addr", srv.Addr, "api_url", cfg.APIURL)
		errc <- fmt.Errorf("web server: %w", srv.ListenAndServe())
	}()
	return <-errc
}
