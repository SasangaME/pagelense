// Package serve runs HTTP servers and shuts them down gracefully.
package serve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Server is an HTTP server with a name for the logs.
type Server struct {
	Name string
	*http.Server
}

// Run serves until ctx is done or a server fails, then shuts all of them down.
func Run(ctx context.Context, log *slog.Logger, timeout time.Duration, servers ...Server) error {
	errc := make(chan error, len(servers))
	for _, s := range servers {
		log.Info("starting "+s.Name+" server", "addr", s.Addr)
		go func() {
			if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("%s server: %w", s.Name, err)
			}
		}()
	}

	var serveErr error
	select {
	case <-ctx.Done():
		log.Info("shutting down", "timeout", timeout.String())
	case serveErr = <-errc:
		log.Error("server failed, shutting down", "error", serveErr)
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	errs := []error{serveErr}
	for _, s := range servers {
		if err := s.Shutdown(ctx); err != nil {
			_ = s.Close()
			errs = append(errs, fmt.Errorf("%s server: shutdown: %w", s.Name, err))
			continue
		}
		log.Info(s.Name + " server stopped")
	}
	return errors.Join(errs...)
}
