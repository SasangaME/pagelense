package serve

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

var discardLogger = slog.New(slog.DiscardHandler)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func waitUp(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never came up: %v", addr, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func newServer(name string, addr string, h http.HandlerFunc) Server {
	return Server{Name: name, Server: &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: time.Second}}
}

func ok(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }

func start(t *testing.T, ctx context.Context, timeout time.Duration, servers ...Server) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, discardLogger, timeout, servers...) }()
	for _, s := range servers {
		waitUp(t, s.Addr)
	}
	return done
}

func wait(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
		return nil
	}
}

func TestRunServesAndStopsCleanly(t *testing.T) {
	app, admin := freeAddr(t), freeAddr(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := start(t, ctx, time.Second, newServer("app", app, ok), newServer("admin", admin, ok))

	for _, addr := range []string{app, admin} {
		resp, err := http.Get("http://" + addr)
		if err != nil {
			t.Fatalf("GET %s: %v", addr, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d", addr, resp.StatusCode)
		}
	}

	cancel()
	if err := wait(t, done); err != nil {
		t.Fatalf("Run = %v, want nil after a clean shutdown", err)
	}
	for _, addr := range []string{app, admin} {
		if conn, err := net.Dial("tcp", addr); err == nil {
			_ = conn.Close()
			t.Errorf("%s still accepts connections after shutdown", addr)
		}
	}
}

func TestRunLetsInFlightRequestsFinish(t *testing.T) {
	addr := freeAddr(t)
	started, release := make(chan struct{}), make(chan struct{})
	slow := func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := start(t, ctx, 5*time.Second, newServer("app", addr, slow))

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + addr)
		if err != nil {
			got <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		got <- result{string(b), err}
	}()

	<-started
	cancel()
	time.Sleep(50 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("Run returned %v before the in-flight request finished", err)
	default:
	}
	close(release)

	if r := <-got; r.err != nil || r.body != "finished" {
		t.Errorf("in-flight request = %q, %v; want it to finish", r.body, r.err)
	}
	if err := wait(t, done); err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
}

func TestRunCutsOffRequestsAfterTimeout(t *testing.T) {
	addr := freeAddr(t)
	started := make(chan struct{})
	canceled := make(chan struct{})
	stuck := func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := start(t, ctx, 50*time.Millisecond, newServer("app", addr, stuck))

	go func() {
		resp, err := http.Get("http://" + addr)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	cancel()

	err := wait(t, done)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run = %v, want a shutdown deadline error", err)
	}
	if !strings.Contains(err.Error(), "app server: shutdown") {
		t.Errorf("error %q does not name the server", err)
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Error("the stuck request's context was never canceled")
	}
}

func TestRunPortInUse(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = taken.Close() }()
	free := freeAddr(t)

	err = Run(t.Context(), discardLogger, time.Second,
		newServer("app", free, ok), newServer("admin", taken.Addr().String(), ok))
	if err == nil || !strings.HasPrefix(err.Error(), "admin server:") {
		t.Fatalf("Run = %v, want an admin server listen error", err)
	}
}
