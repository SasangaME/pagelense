package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Recovery logs panics and answers 500 through respond.
func Recovery(log *slog.Logger, respond func(c *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(p)
			}

			log.ErrorContext(c.Request.Context(), "panic",
				slog.String("request_id", RequestIDFrom(c.Request.Context())),
				slog.Any("panic", p),
				slog.String("stack", string(debug.Stack())),
			)
			_ = c.Error(fmt.Errorf("panic: %v", p))

			if c.Writer.Written() {
				c.Abort()
				return
			}
			c.Status(http.StatusInternalServerError)
			respond(c)
			c.Abort()
		}()
		c.Next()
	}
}
