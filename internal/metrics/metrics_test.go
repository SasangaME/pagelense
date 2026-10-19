package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/SasangaME/pagelense/internal/analyzer"
	"github.com/SasangaME/pagelense/internal/api"
	"github.com/SasangaME/pagelense/internal/frontend"
	"github.com/SasangaME/pagelense/internal/linkcheck"
)

// the wrappers must fit where the services use them
var (
	_ api.Analyzer         = InstrumentAnalyzer(prometheus.NewRegistry(), nil)
	_ frontend.Analyzer    = InstrumentBackend(prometheus.NewRegistry(), nil)
	_ analyzer.LinkChecker = InstrumentChecker(prometheus.NewRegistry(), nil)
	_ LinkChecker          = (*linkcheck.Checker)(nil)
	_ Analyzer             = (*frontend.Client)(nil)
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestNewRegistry(t *testing.T) {
	families, err := NewRegistry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	names := make(map[string]bool, len(families))
	for _, f := range families {
		names[f.GetName()] = true
	}
	for _, want := range []string{"go_goroutines", "process_start_time_seconds"} {
		if !names[want] {
			t.Errorf("registry is missing %s", want)
		}
	}
}

func TestHTTPMiddleware(t *testing.T) {
	m := NewHTTP(prometheus.NewRegistry())
	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusTeapot) })

	for _, path := range []string{"/items/1", "/items/2", "/random/path"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	// requests are labeled by route template, so /items/1 and /items/2 share a series
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/items/:id", "418")); got != 2 {
		t.Errorf("route requests = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", unmatchedRoute, "404")); got != 1 {
		t.Errorf("unmatched requests = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(m.requests); got != 2 {
		t.Errorf("request series = %d, want 2", got)
	}
	if got := testutil.CollectAndCount(m.duration); got != 2 {
		t.Errorf("duration series = %d, want 2", got)
	}
}

type fakeAnalyzer struct{ err error }

func (f fakeAnalyzer) Analyze(context.Context, string) (analyzer.Result, error) {
	return analyzer.Result{URL: "https://example.com"}, f.err
}

// observations returns how many observations the histogram name has for the given outcome.
func observations(t *testing.T, reg *prometheus.Registry, name, outcome string) uint64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "outcome" && l.GetValue() == outcome {
					return m.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}

func TestInstrumentAnalyzer(t *testing.T) {
	for _, err := range []error{nil, errors.New("boom"), context.Canceled} {
		reg := prometheus.NewRegistry()
		res, gotErr := InstrumentAnalyzer(reg, fakeAnalyzer{err: err}).Analyze(t.Context(), "https://example.com")

		if !errors.Is(gotErr, err) || res.URL != "https://example.com" {
			t.Errorf("Analyze = %v, %v; want the wrapped analyzer's result and error", res, gotErr)
		}
		if got := observations(t, reg, "pagelense_analysis_duration_seconds", outcome(err)); got != 1 {
			t.Errorf("err %v: %s observations = %d, want 1", err, outcome(err), got)
		}
	}
}

func TestInstrumentBackendErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		outcome    string
		wantStatus string // "" means no error counted
	}{
		{"ok", nil, outcomeOK, ""},
		{"timeout", &frontend.APIError{Status: http.StatusGatewayTimeout, Message: "slow"}, outcomeError, "504"},
		{"backend status", &frontend.APIError{Status: http.StatusNotFound, Message: "upstream 404"}, outcomeError, "404"},
		{"other error", errors.New("encode request"), outcomeError, "502"},
		{"canceled", context.Canceled, outcomeCanceled, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			a, ok := InstrumentBackend(reg, fakeAnalyzer{err: tt.err}).(*timedAnalyzer)
			if !ok {
				t.Fatal("InstrumentBackend did not return a *timedAnalyzer")
			}
			_, _ = a.Analyze(t.Context(), "https://example.com")

			if got := observations(t, reg, "pagelense_backend_request_duration_seconds", tt.outcome); got != 1 {
				t.Errorf("%s observations = %d, want 1", tt.outcome, got)
			}
			errs := a.errors
			wantSeries := 0
			if tt.wantStatus != "" {
				wantSeries = 1
				if got := testutil.ToFloat64(errs.WithLabelValues(tt.wantStatus)); got != 1 {
					t.Errorf("errors{status=%s} = %v, want 1", tt.wantStatus, got)
				}
			}
			if got := testutil.CollectAndCount(errs); got != wantSeries {
				t.Errorf("error series = %d, want %d", got, wantSeries)
			}
		})
	}
}

type fakeChecker struct{ results []linkcheck.Result }

func (f fakeChecker) Check(context.Context, []string) []linkcheck.Result { return f.results }

func TestInstrumentChecker(t *testing.T) {
	results := []linkcheck.Result{
		{URL: "https://a.example", StatusCode: 200},
		{URL: "https://b.example", StatusCode: 301},
		{URL: "https://c.example", StatusCode: 404},
		{URL: "https://d.example", Err: errors.New("dial tcp: connection refused")},
	}
	reg := prometheus.NewRegistry()
	c := InstrumentChecker(reg, fakeChecker{results: results})

	// 6 unique URLs (a.example is listed twice), 4 results: 2 were skipped
	urls := []string{"https://a.example", "https://a.example", "https://b.example", "https://c.example",
		"https://d.example", "https://e.example", "https://f.example"}
	if got := c.Check(t.Context(), urls); len(got) != len(results) {
		t.Fatalf("Check returned %d results, want the wrapped checker's %d", len(got), len(results))
	}

	want := `
# HELP pagelense_link_checks_total Unique links per analysis, by outcome: accessible, inaccessible (4xx/5xx), failed (no response) or skipped (over the cap, budget or canceled).
# TYPE pagelense_link_checks_total counter
pagelense_link_checks_total{outcome="accessible"} 2
pagelense_link_checks_total{outcome="failed"} 1
pagelense_link_checks_total{outcome="inaccessible"} 1
pagelense_link_checks_total{outcome="skipped"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "pagelense_link_checks_total"); err != nil {
		t.Error(err)
	}
}

func TestInstrumentCheckerStartsAtZero(t *testing.T) {
	reg := prometheus.NewRegistry()
	InstrumentChecker(reg, fakeChecker{})
	if got := testutil.CollectAndCount(reg, "pagelense_link_checks_total"); got != 4 {
		t.Errorf("series before any check = %d, want 4", got)
	}
}

func TestMetricNamesLint(t *testing.T) {
	reg := prometheus.NewRegistry()
	NewHTTP(reg)
	InstrumentAnalyzer(reg, nil)
	InstrumentBackend(reg, nil)
	InstrumentChecker(reg, nil)
	problems, err := testutil.GatherAndLint(reg)
	if err != nil {
		t.Fatalf("GatherAndLint: %v", err)
	}
	for _, p := range problems {
		t.Errorf("%s: %s", p.Metric, p.Text)
	}
}
