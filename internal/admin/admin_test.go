package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestHandler(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewCounter(prometheus.CounterOpts{Name: "test_total", Help: "Test counter."}))
	h := Handler(reg)

	tests := []struct {
		method, path string
		want         int
		body         string // substring of the body; "" skips the check
	}{
		{http.MethodGet, "/metrics", http.StatusOK, "test_total 0"},
		{http.MethodGet, "/debug/pprof/", http.StatusOK, "goroutine"},
		{http.MethodGet, "/debug/pprof/goroutine?debug=1", http.StatusOK, "goroutine profile"},
		{http.MethodGet, "/debug/pprof/cmdline", http.StatusOK, ""},
		{http.MethodPost, "/debug/pprof/symbol", http.StatusOK, "num_symbols"},
		{http.MethodPost, "/metrics", http.StatusMethodNotAllowed, ""},
		{http.MethodGet, "/", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
			if !strings.Contains(w.Body.String(), tt.body) {
				t.Errorf("body is missing %q", tt.body)
			}
		})
	}
}

func TestMetricsIncludeScrapeCounters(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(prometheus.NewRegistry()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(w.Body.String(), "promhttp_metric_handler_requests_total") {
		t.Error("/metrics is missing promhttp's own counters")
	}
}

func TestNewServer(t *testing.T) {
	srv := NewServer(9091, prometheus.NewRegistry())
	if srv.Addr != ":9091" {
		t.Errorf("Addr = %q, want :9091", srv.Addr)
	}
	// a 30s CPU profile, pprof's default, must fit in the write timeout
	if srv.WriteTimeout <= 30e9 {
		t.Errorf("WriteTimeout = %v, want more than 30s", srv.WriteTimeout)
	}
}
