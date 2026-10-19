package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func testOptions() Options {
	return Options{
		Timeout:      2 * time.Second,
		MaxRedirects: 3,
		MaxBodyBytes: 1024,
		UserAgent:    "pagelense-test/1.0",
		// httptest servers listen on loopback
		AllowPrivateIPs: true,
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

func TestFetchNon2xx(t *testing.T) {
	tests := []struct {
		code    int
		wantMsg string
	}{
		{code: http.StatusNotFound, wantMsg: "returned 404 Not Found"},
		{code: http.StatusInternalServerError, wantMsg: "returned 500 Internal Server Error"},
		{code: http.StatusServiceUnavailable, wantMsg: "returned 503 Service Unavailable"},
		{code: 599, wantMsg: "returned 599"},
	}

	for _, tt := range tests {
		t.Run(tt.wantMsg, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
			}))
			defer srv.Close()

			_, err := New(testOptions()).Fetch(context.Background(), srv.URL)
			fe := requireFetchError(t, err, tt.code)
			// the message names the page's host (host:port for httptest)
			if want := strings.TrimPrefix(srv.URL, "http://") + " " + tt.wantMsg; fe.Message != want {
				t.Errorf("Message = %q, want %q", fe.Message, want)
			}
		})
	}
}

func TestFetchConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close() // nothing listens on addr any more

	_, err := New(testOptions()).Fetch(context.Background(), addr)
	fe := requireFetchError(t, err, http.StatusBadGateway)
	if !strings.HasPrefix(fe.Message, "connection refused by ") {
		t.Errorf("Message = %q", fe.Message)
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
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html")
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
		_ = requireFetchError(t, err, http.StatusBadGateway)
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
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(strings.Repeat("a", tt.size)))
			}))
			defer srv.Close()

			page, err := New(testOptions()).Fetch(context.Background(), srv.URL)
			if tt.wantErr {
				if !errors.Is(err, ErrBodyTooLarge) {
					t.Fatalf("error = %v, want ErrBodyTooLarge", err)
				}
				_ = requireFetchError(t, err, http.StatusBadGateway)
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

func TestFetchContentType(t *testing.T) {
	tests := []struct {
		name    string
		header  string // empty: no Content-Type header, so the body is sniffed
		body    string
		wantErr bool
	}{
		{name: "html", header: "text/html"},
		{name: "html with charset", header: "text/html; charset=ISO-8859-1"},
		{name: "html uppercase", header: "TEXT/HTML"},
		{name: "xhtml", header: "application/xhtml+xml"},
		{name: "json", header: "application/json", wantErr: true},
		{name: "plain text", header: "text/plain", wantErr: true},
		{name: "image", header: "image/png", wantErr: true},
		{name: "pdf", header: "application/pdf", wantErr: true},
		{name: "xml", header: "application/xml", wantErr: true},
		{name: "malformed", header: "text/html;;;=", wantErr: true},
		{name: "missing, sniffed as html", body: "<!DOCTYPE html><title>x</title>"},
		{name: "missing, sniffed as text", body: "just some text", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.header != "" {
					w.Header().Set("Content-Type", tt.header)
				} else {
					w.Header()["Content-Type"] = nil // stop net/http from sniffing on the server side
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			page, err := New(testOptions()).Fetch(context.Background(), srv.URL)
			if tt.wantErr {
				if !errors.Is(err, ErrNotHTML) {
					t.Fatalf("error = %v, want ErrNotHTML", err)
				}
				fe := requireFetchError(t, err, http.StatusUnsupportedMediaType)
				if !strings.Contains(fe.Message, "is not HTML") {
					t.Errorf("Message = %q", fe.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if string(page.Body) != tt.body {
				t.Errorf("Body = %q, want %q", page.Body, tt.body)
			}
		})
	}
}

func TestFetchNonHTMLBodyNotRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(strings.Repeat("a", 1<<20))) // over MaxBodyBytes
	}))
	defer srv.Close()

	_, err := New(testOptions()).Fetch(context.Background(), srv.URL)
	if !errors.Is(err, ErrNotHTML) {
		t.Fatalf("error = %v, want ErrNotHTML before the size check", err)
	}
}

func TestFetchBodyReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("<html>")) // connection closes before 100 bytes arrive
	}))
	defer srv.Close()

	_, err := New(testOptions()).Fetch(context.Background(), srv.URL)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want io.ErrUnexpectedEOF", err)
	}
	fe := requireFetchError(t, err, http.StatusBadGateway)
	if !strings.HasPrefix(fe.Message, "could not fetch ") {
		t.Errorf("Message = %q", fe.Message)
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
	_ = requireFetchError(t, err, http.StatusGatewayTimeout)
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
	var fe *FetchError
	if errors.As(err, &fe) {
		t.Errorf("caller cancellation should not be a FetchError, got %v", fe)
	}
}

func TestTransportError(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{
			name:     "DNS failure",
			err:      &url.Error{Op: "Get", URL: "http://nope.invalid", Err: &net.OpError{Op: "dial", Err: &net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true}}},
			wantCode: http.StatusBadGateway,
			wantMsg:  "could not resolve host example.com",
		},
		{
			name:     "DNS timeout",
			err:      &net.DNSError{Err: "i/o timeout", Name: "slow.example", IsTimeout: true},
			wantCode: http.StatusGatewayTimeout,
			wantMsg:  "timed out fetching example.com",
		},
		{
			name:     "connection refused",
			err:      &url.Error{Op: "Get", URL: "http://example.com", Err: refused},
			wantCode: http.StatusBadGateway,
			wantMsg:  "connection refused by example.com",
		},
		{
			name:     "deadline exceeded",
			err:      fmt.Errorf("read body: %w", context.DeadlineExceeded),
			wantCode: http.StatusGatewayTimeout,
			wantMsg:  "timed out fetching example.com",
		},
		{
			name:     "other",
			err:      errors.New("tls: handshake failure"),
			wantCode: http.StatusBadGateway,
			wantMsg:  "could not fetch example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := transportError("example.com", tt.err)
			fe := requireFetchError(t, err, tt.wantCode)
			if fe.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", fe.Message, tt.wantMsg)
			}
			if !errors.Is(err, tt.err) {
				t.Errorf("FetchError does not unwrap to the cause")
			}
		})
	}

	t.Run("caller cancellation passes through", func(t *testing.T) {
		cause := fmt.Errorf("get: %w", context.Canceled)
		//nolint:errorlint // checking identity: the cause must be returned unwrapped
		if err := transportError("example.com", cause); err != cause {
			t.Errorf("error = %v, want the cause unchanged", err)
		}
	})
}

func TestFetchErrorString(t *testing.T) {
	if got := statusError("example.com", http.StatusNotFound).Error(); got != "example.com returned 404 Not Found" {
		t.Errorf("without cause: %q", got)
	}
	err := &FetchError{StatusCode: http.StatusBadGateway, Message: "could not fetch example.com", Err: errors.New("boom")}
	if got := err.Error(); got != "could not fetch example.com: boom" {
		t.Errorf("with cause: %q", got)
	}
}

func requireFetchError(t *testing.T, err error, wantCode int) *FetchError {
	t.Helper()
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("error = %v (%T), want *FetchError", err, err)
	}
	if fe.StatusCode != wantCode {
		t.Errorf("StatusCode = %d, want %d", fe.StatusCode, wantCode)
	}
	return fe
}

func TestFetchInvalidURL(t *testing.T) {
	_, err := New(testOptions()).Fetch(context.Background(), "http://exa mple.com")
	if err == nil {
		t.Fatal("expected an error for an invalid URL")
	}
}
