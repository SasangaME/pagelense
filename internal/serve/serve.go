// Package serve runs a service's HTTP servers and shuts them down gracefully.
package serve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server is an HTTP server with a name for the logs, such as "api" or "admin".
type Server struct {
	Name string
	*http.Server
}

// Run serves every server until ctx is done or one fails, then shuts all down, giving in-flight requests up to timeout; it returns nil after a clean shutdown on ctx.
func Run(ctx context.Context, log *slog.Logger, timeout time.Duration, servers ...Server) error {
	// listen first, so a port already in use stops startup before anything is served
	var lc net.ListenConfig
	listeners := make([]net.Listener, 0, len(servers))
	for _, s := range servers {
		ln, err := lc.Listen(ctx, "tcp", s.Addr)
		if err != nil {
			for _, l := range listeners {
				_ = l.Close()
			}
			return fmt.Errorf("%s server: %w", s.Name, err)
		}
		listeners = append(listeners, ln)
	}
	return serve(ctx, log, timeout, servers, listeners)
}

// serve runs each server on its listener until ctx is done or one fails, then shuts them all down.
func serve(ctx context.Context, log *slog.Logger, timeout time.Duration, servers []Server, listeners []net.Listener) error {
	// buffered, so a server failing after shutdown has started never blocks its goroutine
	errc := make(chan error, len(servers))
	for i, s := range servers {
		log.Info(s.Name+" listening", "addr", listeners[i].Addr().String())
		go func() {
			if err := s.Serve(listeners[i]); !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("%s server: %w", s.Name, err)
			}
		}()
	}

	var serveErr error
	select {
	case <-ctx.Done():
		log.Info("shutting down", "timeout", timeout.String())
	case serveErr = <-errc:
		log.Error("server failed, shutting down", "error", serveErr, "timeout", timeout.String())
	}
	return errors.Join(serveErr, shutdown(ctx, log, timeout, servers))
}

// shutdown stops every server at once, so they share one timeout rather than adding theirs up.
func shutdown(ctx context.Context, log *slog.Logger, timeout time.Duration, servers []Server) error {
	// ctx is usually canceled by now, so only its values carry over
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	errs := make([]error, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			if err := s.Shutdown(ctx); err != nil {
				// requests still running are cut off; their contexts are canceled with the connection
				_ = s.Close()
				errs[i] = fmt.Errorf("%s server: shutdown: %w", s.Name, err)
				return
			}
			log.Info(s.Name+" stopped", "duration_ms", time.Since(start).Milliseconds())
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}
