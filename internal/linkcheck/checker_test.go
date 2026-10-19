package linkcheck

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SasangaME/pagelense/internal/fetcher"
)

func distinctURLs(base string, n int) []string {
	urls := make([]string, n)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/%d", base, i)
	}
	return urls
}

func TestCheckStatuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
		case "/moved":
			http.Redirect(w, r, "/ok", http.StatusMovedPermanently)
		}
	}))
	defer srv.Close()

	tests := []struct {
		path       string
		wantStatus int
		wantOK     bool
	}{
		{"/ok", http.StatusOK, true},
		{"/moved", http.StatusOK, true}, // redirects are followed
		{"/missing", http.StatusNotFound, false},
		{"/error", http.StatusInternalServerError, false},
	}
	urls := make([]string, len(tests))
	for i, tt := range tests {
		urls[i] = srv.URL + tt.path
	}
	results := New(srv.Client(), Options{Workers: 2}).Check(context.Background(), urls)

	if len(results) != len(urls) {
		t.Fatalf("got %d results, want %d", len(results), len(urls))
	}
	byURL := make(map[string]Result, len(results))
	for _, r := range results {
		byURL[r.URL] = r
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			r, ok := byURL[srv.URL+tt.path]
			if !ok {
				t.Fatal("no result")
			}
			if r.StatusCode != tt.wantStatus || r.Accessible() != tt.wantOK || r.Err != nil {
				t.Errorf("got status %d, accessible %v, err %v; want status %d, accessible %v",
					r.StatusCode, r.Accessible(), r.Err, tt.wantStatus, tt.wantOK)
			}
		})
	}
}

func TestCheckMethodFallback(t *testing.T) {
	tests := []struct {
		name        string
		headStatus  int
		getStatus   int
		wantMethods []string
		wantStatus  int
	}{
		{"HEAD allowed", http.StatusOK, 0, []string{http.MethodHead}, http.StatusOK},
		{"GET after 405", http.StatusMethodNotAllowed, http.StatusOK, []string{http.MethodHead, http.MethodGet}, http.StatusOK},
		{"GET after 405 fails", http.StatusMethodNotAllowed, http.StatusNotFound, []string{http.MethodHead, http.MethodGet}, http.StatusNotFound},
		{"GET after HEAD 404", http.StatusNotFound, http.StatusOK, []string{http.MethodHead, http.MethodGet}, http.StatusOK},
		{"GET after HEAD 404 also fails", http.StatusNotFound, http.StatusNotFound, []string{http.MethodHead, http.MethodGet}, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var methods []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				methods = append(methods, r.Method)
				mu.Unlock()
				if r.Method == http.MethodHead {
					w.WriteHeader(tt.headStatus)
					return
				}
				w.WriteHeader(tt.getStatus)
			}))
			defer srv.Close()

			results := New(srv.Client(), Options{Workers: 1}).Check(context.Background(), []string{srv.URL})
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			if results[0].StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", results[0].StatusCode, tt.wantStatus)
			}
			if !slices.Equal(methods, tt.wantMethods) {
				t.Errorf("methods = %v, want %v", methods, tt.wantMethods)
			}
		})
	}
}

func TestCheckNoURLs(t *testing.T) {
	results := New(http.DefaultClient, Options{Workers: 4}).Check(context.Background(), nil)
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
}

func TestCheckNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	results := New(http.DefaultClient, Options{Workers: 1}).Check(context.Background(), []string{url})
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Err == nil || results[0].Accessible() {
		t.Errorf("got %+v, want a network error", results[0])
	}
}

func TestCheckWorkerCount(t *testing.T) {
	const workers = 3
	var inFlight, peak atomic.Int32
	release := make(chan struct{})
	var once sync.Once

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		if n == workers {
			once.Do(func() { close(release) })
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	urls := distinctURLs(srv.URL, 10)
	results := New(srv.Client(), Options{Workers: workers}).Check(context.Background(), urls)

	if len(results) != len(urls) {
		t.Fatalf("got %d results, want %d", len(results), len(urls))
	}
	if got := peak.Load(); got != workers {
		t.Errorf("peak concurrency = %d, want %d", got, workers)
	}
}

func TestCheckCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		if calls.Add(1) == 1 {
			cancel()
		}
	}))
	defer srv.Close()

	urls := distinctURLs(srv.URL, 50)

	done := make(chan []Result)
	go func() { done <- New(srv.Client(), Options{Workers: 1}).Check(ctx, urls) }()

	select {
	case results := <-done:
		if len(results) >= len(urls) {
			t.Errorf("got %d results, want fewer than %d after cancel", len(results), len(urls))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Check did not return after cancel")
	}
}

func blockUntilDone(_ http.ResponseWriter, r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(2 * time.Second):
	}
}

func TestCheckTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(blockUntilDone))
	defer srv.Close()

	start := time.Now()
	results := New(srv.Client(), Options{Workers: 1, Timeout: 50 * time.Millisecond}).
		Check(context.Background(), []string{srv.URL})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Check took %v, want about 50ms", elapsed)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !errors.Is(results[0].Err, context.DeadlineExceeded) || results[0].Accessible() {
		t.Errorf("got %+v, want a deadline error", results[0])
	}
}

func TestCheckTimeoutIsPerLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(20 * time.Millisecond)
	}))
	defer srv.Close()

	urls := distinctURLs(srv.URL, 15)
	results := New(srv.Client(), Options{Workers: 1, Timeout: 200 * time.Millisecond}).
		Check(context.Background(), urls)

	for _, r := range results {
		if !r.Accessible() {
			t.Errorf("got %+v, want accessible", r)
		}
	}
}

func TestCheckCanceledInFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	arrived := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(arrived) })
		blockUntilDone(w, r)
	}))
	defer srv.Close()

	go func() {
		<-arrived
		cancel()
	}()

	start := time.Now()
	results := New(srv.Client(), Options{Workers: 1}).Check(ctx, distinctURLs(srv.URL, 2))
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Check took %v after cancel, want it to return promptly", elapsed)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (second URL should be skipped)", len(results))
	}
	if !errors.Is(results[0].Err, context.Canceled) {
		t.Errorf("got %+v, want context.Canceled", results[0])
	}
}

func TestCheckBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			blockUntilDone(w, r)
		}
	}))
	defer srv.Close()

	urls := append([]string{srv.URL + "/fast", srv.URL + "/slow"}, distinctURLs(srv.URL+"/queued", 5)...)
	start := time.Now()
	results := New(srv.Client(), Options{Workers: 1, Budget: 100 * time.Millisecond}).
		Check(context.Background(), urls)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Check took %v, want about the 100ms budget", elapsed)
	}
	if len(results) != 1 || results[0].URL != srv.URL+"/fast" || !results[0].Accessible() {
		t.Errorf("got %+v, want only the accessible /fast result", results)
	}
}

func TestCheckBudgetKeepsLinkTimeouts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(blockUntilDone))
	defer srv.Close()

	results := New(srv.Client(), Options{Workers: 1, Timeout: 50 * time.Millisecond, Budget: time.Second}).
		Check(context.Background(), []string{srv.URL})
	if len(results) != 1 || !errors.Is(results[0].Err, context.DeadlineExceeded) {
		t.Errorf("got %+v, want one deadline error", results)
	}
}

func TestCheckUserAgent(t *testing.T) {
	tests := []struct {
		name, userAgent, want string
	}{
		{"set", "pagelense-test/1.0", "pagelense-test/1.0"},
		{"unset keeps Go's default", "", "Go-http-client/1.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got atomic.Value
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got.Store(r.UserAgent())
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusMethodNotAllowed)
				} else if r.UserAgent() != tt.want {
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()

			results := New(srv.Client(), Options{UserAgent: tt.userAgent}).Check(context.Background(), []string{srv.URL})
			if got.Load() != tt.want || len(results) != 1 || !results[0].Accessible() {
				t.Errorf("server saw User-Agent %v, results %+v; want %q on HEAD and GET", got.Load(), results, tt.want)
			}
		})
	}
}

func TestUnique(t *testing.T) {
	tests := []struct {
		name  string
		urls  []string
		limit int
		want  []string
	}{
		{"empty", nil, 0, []string{}},
		{"no duplicates", []string{"a", "b"}, 0, []string{"a", "b"}},
		{"duplicates keep first order", []string{"b", "a", "b", "c", "a"}, 0, []string{"b", "a", "c"}},
		{"limit", []string{"a", "b", "c"}, 2, []string{"a", "b"}},
		{"limit counts unique URLs", []string{"a", "a", "a", "b", "c"}, 2, []string{"a", "b"}},
		{"limit above count", []string{"a", "b"}, 5, []string{"a", "b"}},
		{"negative limit means none", []string{"a", "b"}, -1, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unique(tt.urls, tt.limit); !slices.Equal(got, tt.want) {
				t.Errorf("unique(%v, %d) = %v, want %v", tt.urls, tt.limit, got, tt.want)
			}
		})
	}
}

func TestCheckDedupAndCap(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
	}))
	defer srv.Close()

	urls := []string{srv.URL + "/a", srv.URL + "/a", srv.URL + "/b", srv.URL + "/a", srv.URL + "/c", srv.URL + "/d"}
	results := New(srv.Client(), Options{Workers: 2, MaxLinks: 3}).Check(context.Background(), urls)

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	want := map[string]int{"/a": 1, "/b": 1, "/c": 1}
	if !maps.Equal(hits, want) {
		t.Errorf("requests per path = %v, want %v", hits, want)
	}
}

func TestCheckSharesFetcherClient(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	f := fetcher.New(fetcher.Options{Timeout: time.Second, MaxRedirects: 3, MaxBodyBytes: 1024})
	results := New(f.Client(), Options{Workers: 1}).Check(context.Background(), []string{srv.URL})

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !errors.Is(results[0].Err, fetcher.ErrBlockedAddress) || results[0].Accessible() {
		t.Errorf("got %+v, want ErrBlockedAddress", results[0])
	}
	if hits.Load() != 0 {
		t.Error("request reached the blocked server")
	}
}
