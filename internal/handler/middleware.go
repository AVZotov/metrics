package handler

import (
	"compress/gzip"
	"net/http"
	"strings"
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

type responseCompressedWriter struct {
	responseWriter
	gw *gzip.Writer
}

func (w *responseCompressedWriter) Write(b []byte) (int, error) {
	return w.gw.Write(b)
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

func ContentTypeMiddleware(contentType string) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.Header.Get("Content-Type"), contentType) {
					w.WriteHeader(http.StatusUnsupportedMediaType)
					return
				}
				next.ServeHTTP(w, r)
			},
		)
	}
}

func CompressMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				cEnc := r.Header.Get("Content-Encoding")
				aEnc := r.Header.Get("Accept-Encoding")
				if strings.Contains(cEnc, "gzip") {
					gr, err := gzip.NewReader(r.Body)
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					defer gr.Close()
					r.Body = gr
				}
				if strings.Contains(aEnc, "gzip") {
					gw := gzip.NewWriter(w)
					defer gw.Close()
					w.Header().Set("Content-Encoding", "gzip")
					ww := &responseCompressedWriter{
						responseWriter: responseWriter{
							ResponseWriter: w,
							status:         http.StatusOK,
						},
						gw: gw,
					}
					next.ServeHTTP(ww, r)
					return
				}
				next.ServeHTTP(w, r)
			},
		)
	}
}
