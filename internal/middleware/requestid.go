// Package middleware provides the Gin middleware shared by the backend and frontend: request IDs, request logging and panic recovery.
package middleware

import (
	"context"
	"crypto/rand"

	"github.com/gin-gonic/gin"
)

// HeaderRequestID carries the request ID between clients, the frontend and the backend.
const HeaderRequestID = "X-Request-ID"

// maxRequestIDLen bounds an incoming ID, so a client cannot fill the logs with a huge header.
const maxRequestIDLen = 128

type requestIDKey struct{}

// RequestID reuses a valid incoming X-Request-ID or generates one, echoes it, and stores it in the request context.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if !validRequestID(id) {
			id = rand.Text()
		}
		c.Header(HeaderRequestID, id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey{}, id))
		c.Next()
	}
}

// RequestIDFrom returns the request ID stored by RequestID, or "" if there is none.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// validRequestID accepts short IDs made of letters, digits and -_.: (UUIDs, ALB trace IDs and our own IDs all fit).
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}
