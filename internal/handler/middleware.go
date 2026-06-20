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
	http.ResponseWriter
	gw      *gzip.Writer
	checked bool
	enabled bool
}

func (w *responseCompressedWriter) checkContentType() {
	if w.checked {
		return
	}
	w.checked = true
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") || strings.Contains(ct, "text/html") {
		w.gw = gzip.NewWriter(w.ResponseWriter)
		w.Header().Set("Content-Encoding", "gzip")
		w.enabled = true
	}
}

func (w *responseCompressedWriter) WriteHeader(status int) {
	w.checkContentType()
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseCompressedWriter) Write(b []byte) (int, error) {
	w.checkContentType()
	if w.enabled {
		return w.gw.Write(b)
	}
	return w.ResponseWriter.Write(b)
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
					cw := &responseCompressedWriter{ResponseWriter: w}
					defer func() {
						if cw.gw != nil {
							cw.gw.Close()
						}
					}()
					next.ServeHTTP(cw, r)
					return
				}
				next.ServeHTTP(w, r)
			},
		)
	}
}
