package handler

import (
	"net/http"
	"time"
	
	"go.uber.org/zap"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	size, err := w.ResponseWriter.Write(b)
	w.size += size
	return size, err
}

func LoggingMiddleware(l *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()
				ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
				next.ServeHTTP(ww, r)
				l.Info(
					"request",
					zap.String("uri", r.RequestURI),
					zap.String("method", r.Method),
					zap.Int("size", ww.size),
					zap.Int("status", ww.status),
					zap.Duration("duration", time.Since(start)),
				)
			},
		)
	}
}
