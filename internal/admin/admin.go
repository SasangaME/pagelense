// Package admin serves /metrics and pprof on a separate port.
package admin

import (
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewServer returns the admin server listening on port.
func NewServer(port int, reg *prometheus.Registry) *http.Server {
	return &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           Handler(reg),
		ReadHeaderTimeout: 5 * time.Second,
		// CPU profiles and traces run for 30s by default
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// Handler returns the admin routes for reg.
func Handler(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	metrics := promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
	mux.Handle("GET /metrics", promhttp.InstrumentMetricHandler(reg, metrics))
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}
