// Package admin serves /metrics and /debug/pprof on a separate port, kept off the public app port.
package admin

import (
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewServer returns the admin server for port: /metrics from reg, and the pprof endpoints.
func NewServer(port int, reg *prometheus.Registry) *http.Server {
	return &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           Handler(reg),
		ReadHeaderTimeout: 5 * time.Second,
		// /debug/pprof/profile and /trace run for ?seconds= (30 by default) and refuse to run past this
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// Handler routes the admin endpoints on its own mux, so nothing registered on http.DefaultServeMux is served.
func Handler(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	// like promhttp.Handler, but for reg instead of the global registry; also counts scrapes and their errors
	metrics := promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
	mux.Handle("GET /metrics", promhttp.InstrumentMetricHandler(reg, metrics))
	mux.HandleFunc("GET /debug/pprof/", pprof.Index) // also serves heap, goroutine, block, mutex, allocs, threadcreate
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol) // go tool pprof looks up symbols with a POST
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}
