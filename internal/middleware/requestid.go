// Package middleware has the Gin middleware both services use.
package middleware

import (
	"context"
	"crypto/rand"

	"github.com/gin-gonic/gin"
)

// HeaderRequestID is the request ID header.
const HeaderRequestID = "X-Request-ID"

type requestIDKey struct{}

// RequestID reuses an incoming X-Request-ID or generates one.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = rand.Text()
		}
		c.Header(HeaderRequestID, id)
		c.Request = c.Request.WithContext(WithRequestID(c.Request.Context(), id))
		c.Next()
	}
}

// WithRequestID returns a copy of ctx carrying id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom returns the request ID in ctx, or "".
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
