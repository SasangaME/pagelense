// Package metrics defines the Prometheus collectors for the application.
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

// unmatchedRoute labels requests no route matched, so unknown paths cannot create new series.
const unmatchedRoute = "unmatched"

// Outcome label values.
const (
	outcomeOK       = "ok"
	outcomeError    = "error"
	outcomeCanceled = "canceled" // the caller went away; says nothing about the service
)

// Link check outcome label values.
const (
	linkAccessible   = "accessible"   // answered below 400
	linkInaccessible = "inaccessible" // answered 400 or above
	linkFailed       = "failed"       // no response: network error or per-link timeout
	linkSkipped      = "skipped"      // over MAX_LINKS, or cut by LINK_BUDGET or cancellation
)

// NewRegistry returns a registry with the Go runtime and process collectors, one per service.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
}

// HTTP counts and times the requests a service handles.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTP registers the request collectors with reg.
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
			// an analysis takes up to FETCH_TIMEOUT + LINK_BUDGET (20s by default); everything else is milliseconds
			Buckets: []float64{.005, .01, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 30},
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

// Middleware records each request under its route template (e.g. /api/v1/analyze), not the raw path.
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

// Analyzer analyzes a web page; both services' handlers depend on this method.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analyzer.Result, error)
}

// LinkChecker checks links; *linkcheck.Checker implements it.
type LinkChecker interface {
	Check(ctx context.Context, urls []string) []linkcheck.Result
}

// timedAnalyzer records how long each Analyze call takes, by outcome.
type timedAnalyzer struct {
	next     Analyzer
	duration *prometheus.HistogramVec
	errors   *prometheus.CounterVec // nil when the caller does not count errors by status
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

// outcome labels an error as ok, canceled or error.
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

// analysisBuckets cover a quick page (under a second) up to the default worst case of FETCH_TIMEOUT + LINK_BUDGET.
var analysisBuckets = []float64{.1, .25, .5, 1, 2.5, 5, 7.5, 10, 15, 20, 30}

// InstrumentAnalyzer wraps the backend's analyzer to record analysis duration, by outcome.
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

// InstrumentBackend wraps the frontend's API client to record call duration by outcome, and errors by status.
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

// backendStatus is the status the frontend shows for err; anything that is not an APIError is shown as 502.
func backendStatus(err error) string {
	var apiErr *frontend.APIError
	if errors.As(err, &apiErr) {
		return strconv.Itoa(apiErr.Status)
	}
	return "502"
}

// countedChecker counts the outcome of every unique link it is asked to check.
type countedChecker struct {
	next   LinkChecker
	checks *prometheus.CounterVec
}

// InstrumentChecker wraps the backend's link checker to count link-check outcomes.
func InstrumentChecker(reg prometheus.Registerer, c LinkChecker) LinkChecker {
	checks := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "link_checks_total",
		Help:      "Unique links per analysis, by outcome: accessible, inaccessible (4xx/5xx), failed (no response) or skipped (over the cap, budget or canceled).",
	}, []string{"outcome"})
	// start every outcome at 0, so rate() works before the first failure
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
	// the checker returns one result per unique URL it finished, so the rest were skipped
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
