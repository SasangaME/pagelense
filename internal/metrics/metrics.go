// Package metrics defines the Prometheus metrics.
package metrics

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/frontend"
	"github.com/SasangaME/pagelense/internal/linkcheck"
)

const namespace = "pagelense"

const unmatchedRoute = "unmatched"

const (
	outcomeOK       = "ok"
	outcomeError    = "error"
	outcomeCanceled = "canceled"
)

const (
	linkAccessible   = "accessible"
	linkInaccessible = "inaccessible"
	linkFailed       = "failed"
	linkSkipped      = "skipped"
)

// NewRegistry returns a registry with the Go and process collectors.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
}

// HTTP counts and times requests.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTP registers the HTTP metrics with reg.
func NewHTTP(reg prometheus.Registerer) *HTTP {
	m := &HTTP{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "HTTP requests handled, by method, route and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "Time to handle an HTTP request, by method and route.",
			Buckets:   []float64{.005, .01, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 30},
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

// Middleware records each request by route template.
func (m *HTTP) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = unmatchedRoute
		}
		method := c.Request.Method
		m.requests.WithLabelValues(method, route, strconv.Itoa(c.Writer.Status())).Inc()
		m.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
}

// Analyzer analyzes a web page.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analyzer.Result, error)
}

// LinkChecker checks links.
type LinkChecker interface {
	Check(ctx context.Context, urls []string) []linkcheck.Result
}

type timedAnalyzer struct {
	next     Analyzer
	duration *prometheus.HistogramVec
	errors   *prometheus.CounterVec // nil for the backend
}

func (t *timedAnalyzer) Analyze(ctx context.Context, rawURL string) (analyzer.Result, error) {
	start := time.Now()
	res, err := t.next.Analyze(ctx, rawURL)
	t.duration.WithLabelValues(outcome(err)).Observe(time.Since(start).Seconds())
	if t.errors != nil && err != nil && !errors.Is(err, context.Canceled) {
		t.errors.WithLabelValues(backendStatus(err)).Inc()
	}
	return res, err
}

func outcome(err error) string {
	switch {
	case err == nil:
		return outcomeOK
	case errors.Is(err, context.Canceled):
		return outcomeCanceled
	default:
		return outcomeError
	}
}

var analysisBuckets = []float64{.1, .25, .5, 1, 2.5, 5, 7.5, 10, 15, 20, 30}

// InstrumentAnalyzer records how long each analysis takes.
func InstrumentAnalyzer(reg prometheus.Registerer, a Analyzer) Analyzer {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "analysis_duration_seconds",
		Help:      "Time to analyze a page (fetch, parse and link checks), by outcome: ok, error or canceled.",
		Buckets:   analysisBuckets,
	}, []string{"outcome"})
	reg.MustRegister(duration)
	return &timedAnalyzer{next: a, duration: duration}
}

// InstrumentBackend records backend call durations and errors.
func InstrumentBackend(reg prometheus.Registerer, a Analyzer) Analyzer {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "backend_request_duration_seconds",
		Help:      "Time for a call to the backend API, by outcome: ok, error or canceled.",
		Buckets:   analysisBuckets,
	}, []string{"outcome"})
	errs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "backend_errors_total",
		Help:      "Failed backend API calls, by the status shown to the user (502 unreachable, 504 timeout, or the backend's own status).",
	}, []string{"status"})
	reg.MustRegister(duration, errs)
	return &timedAnalyzer{next: a, duration: duration, errors: errs}
}

func backendStatus(err error) string {
	var apiErr *frontend.APIError
	if errors.As(err, &apiErr) {
		return strconv.Itoa(apiErr.Status)
	}
	return "502"
}

type countedChecker struct {
	next   LinkChecker
	checks *prometheus.CounterVec
}

// InstrumentChecker counts link check outcomes.
func InstrumentChecker(reg prometheus.Registerer, c LinkChecker) LinkChecker {
	checks := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "link_checks_total",
		Help:      "Unique links per analysis, by outcome: accessible, inaccessible (4xx/5xx), failed (no response) or skipped (over the cap, budget or canceled).",
	}, []string{"outcome"})
	// export every outcome from the start
	for _, o := range []string{linkAccessible, linkInaccessible, linkFailed, linkSkipped} {
		checks.WithLabelValues(o)
	}
	reg.MustRegister(checks)
	return &countedChecker{next: c, checks: checks}
}

func (c *countedChecker) Check(ctx context.Context, urls []string) []linkcheck.Result {
	results := c.next.Check(ctx, urls)
	for _, r := range results {
		c.checks.WithLabelValues(linkOutcome(r)).Inc()
	}
	// links without a result were skipped
	if skipped := countUnique(urls) - len(results); skipped > 0 {
		c.checks.WithLabelValues(linkSkipped).Add(float64(skipped))
	}
	return results
}

func linkOutcome(r linkcheck.Result) string {
	switch {
	case r.Err != nil:
		return linkFailed
	case r.Accessible():
		return linkAccessible
	default:
		return linkInaccessible
	}
}

func countUnique(urls []string) int {
	seen := make(map[string]struct{}, len(urls))
	for _, u := range urls {
		seen[u] = struct{}{}
	}
	return len(seen)
}
