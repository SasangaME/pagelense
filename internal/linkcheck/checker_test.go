package linkcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckAllURLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	urls := []string{srv.URL + "/a", srv.URL + "/b", srv.URL + "/missing", srv.URL + "/c"}
	results := New(srv.Client(), 2).Check(context.Background(), urls)

	if len(results) != len(urls) {
		t.Fatalf("got %d results, want %d", len(results), len(urls))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].URL < results[j].URL })
	for _, r := range results {
		wantOK := r.URL != srv.URL+"/missing"
		if r.Accessible() != wantOK {
			t.Errorf("%s: Accessible() = %v, want %v (status %d, err %v)", r.URL, r.Accessible(), wantOK, r.StatusCode, r.Err)
		}
	}
}

func TestCheckMethodFallback(t *testing.T) {
	tests := []struct {
		name        string
		getStatus   int
		wantMethods []string
		wantStatus  int
	}{
		{"HEAD allowed", 0, []string{http.MethodHead}, http.StatusOK},
		{"GET after 405", http.StatusOK, []string{http.MethodHead, http.MethodGet}, http.StatusOK},
		{"GET after 405 fails", http.StatusNotFound, []string{http.MethodHead, http.MethodGet}, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var methods []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				methods = append(methods, r.Method)
				mu.Unlock()
				if tt.getStatus == 0 {
					return // 200 for every method
				}
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.WriteHeader(tt.getStatus)
			}))
			defer srv.Close()

			results := New(srv.Client(), 1).Check(context.Background(), []string{srv.URL})
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
	results := New(http.DefaultClient, 4).Check(context.Background(), nil)
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
}

func TestCheckNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens on the port any more

	results := New(http.DefaultClient, 1).Check(context.Background(), []string{url})
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
	// every request waits until `workers` requests are running at once
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

	urls := make([]string, 10)
	for i := range urls {
		urls[i] = srv.URL
	}
	results := New(srv.Client(), workers).Check(context.Background(), urls)

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

	urls := make([]string, 50)
	for i := range urls {
		urls[i] = srv.URL
	}

	done := make(chan []Result)
	go func() { done <- New(srv.Client(), 1).Check(ctx, urls) }()

	select {
	case results := <-done:
		if len(results) >= len(urls) {
			t.Errorf("got %d results, want fewer than %d after cancel", len(results), len(urls))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Check did not return after cancel")
	}
}
