// Package serve runs HTTP servers and shuts them down gracefully.
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

// Server is an HTTP server with a name for the logs.
type Server struct {
	Name string
	*http.Server
}

// Run serves until ctx is done or a server fails, then shuts all of them down.
func Run(ctx context.Context, log *slog.Logger, timeout time.Duration, servers ...Server) error {
	// listen first, so a port in use stops startup
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

func serve(ctx context.Context, log *slog.Logger, timeout time.Duration, servers []Server, listeners []net.Listener) error {
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

func shutdown(ctx context.Context, log *slog.Logger, timeout time.Duration, servers []Server) error {
	// ctx is canceled by now
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
