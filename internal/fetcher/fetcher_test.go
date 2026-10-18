package fetcher

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testOptions() Options {
	return Options{
		Timeout:      2 * time.Second,
		MaxRedirects: 3,
		MaxBodyBytes: 1024,
		UserAgent:    "pagelense-test/1.0",
	}
}

func TestFetchOK(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!DOCTYPE html><title>Hi</title>"))
	}))
	defer srv.Close()

	page, err := New(testOptions()).Fetch(context.Background(), srv.URL+"/page")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if page.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", page.StatusCode)
	}
	if page.ContentType != "text/html; charset=utf-8" {
		t.Errorf("ContentType = %q", page.ContentType)
	}
	if string(page.Body) != "<!DOCTYPE html><title>Hi</title>" {
		t.Errorf("Body = %q", page.Body)
	}
	if page.URL.String() != srv.URL+"/page" {
		t.Errorf("URL = %q, want %q", page.URL, srv.URL+"/page")
	}
	if gotUA != "pagelense-test/1.0" {
		t.Errorf("User-Agent = %q, want pagelense-test/1.0", gotUA)
	}
}

func TestFetchNon2xxReturnsPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	page, err := New(testOptions()).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if page.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", page.StatusCode)
	}
}

func TestFetchRedirects(t *testing.T) {
	var gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/middle", http.StatusFound)
	})
	mux.HandleFunc("/middle", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/final", func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Run("followed within limit", func(t *testing.T) {
		page, err := New(testOptions()).Fetch(context.Background(), srv.URL+"/start")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if page.URL.Path != "/final" {
			t.Errorf("final URL path = %q, want /final", page.URL.Path)
		}
		if gotUA != "pagelense-test/1.0" {
			t.Errorf("User-Agent after redirect = %q", gotUA)
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		opts := testOptions()
		opts.MaxRedirects = 2
		if _, err := New(opts).Fetch(context.Background(), srv.URL+"/start"); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
	})

	t.Run("over limit", func(t *testing.T) {
		opts := testOptions()
		opts.MaxRedirects = 1
		_, err := New(opts).Fetch(context.Background(), srv.URL+"/start")
		if !errors.Is(err, ErrTooManyRedirects) {
			t.Fatalf("error = %v, want ErrTooManyRedirects", err)
		}
	})

	t.Run("loop", func(t *testing.T) {
		_, err := New(testOptions()).Fetch(context.Background(), srv.URL+"/loop")
		if !errors.Is(err, ErrTooManyRedirects) {
			t.Fatalf("error = %v, want ErrTooManyRedirects", err)
		}
	})
}

func TestFetchBodyLimit(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "under limit", size: 100},
		{name: "exactly at limit", size: 1024},
		{name: "one byte over", size: 1025, wantErr: true},
		{name: "far over", size: 1 << 20, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(strings.Repeat("a", tt.size)))
			}))
			defer srv.Close()

			page, err := New(testOptions()).Fetch(context.Background(), srv.URL)
			if tt.wantErr {
				if !errors.Is(err, ErrBodyTooLarge) {
					t.Fatalf("error = %v, want ErrBodyTooLarge", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(page.Body) != tt.size {
				t.Errorf("len(Body) = %d, want %d", len(page.Body), tt.size)
			}
		})
	}
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	opts := testOptions()
	opts.Timeout = 50 * time.Millisecond
	_, err := New(opts).Fetch(context.Background(), srv.URL)

	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("error = %v, want a timeout", err)
	}
}

func TestFetchContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	_, err := New(testOptions()).Fetch(ctx, srv.URL)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestFetchInvalidURL(t *testing.T) {
	_, err := New(testOptions()).Fetch(context.Background(), "http://exa mple.com")
	if err == nil {
		t.Fatal("expected an error for an invalid URL")
	}
}
