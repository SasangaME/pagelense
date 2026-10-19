package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// healthPath is polled by the load balancer every few seconds, so it is logged at debug level only.
const healthPath = "/healthz"

// Logger logs one line per request with any errors recorded through c.Error; 5xx logs as error, 4xx as warn, the rest as info.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		attrs := []slog.Attr{
			slog.String("request_id", RequestIDFrom(c.Request.Context())),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path), // query strings are left out; no route uses them
			slog.Int("status", status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.Int("bytes", max(c.Writer.Size(), 0)), // -1 when nothing was written
			slog.String("client_ip", c.ClientIP()),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.Any("errors", c.Errors.Errors()))
		}

		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		case c.Request.URL.Path == healthPath:
			level = slog.LevelDebug
		}
		log.LogAttrs(c.Request.Context(), level, "request", attrs...)
	}
}
