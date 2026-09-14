package handler

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AVZotov/metrics/internal/pool"
	"github.com/AVZotov/metrics/internal/sign"
	"go.uber.org/zap"
)

// generate:reset
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
	gw      *gzipWriter
	checked bool
	enabled bool
}

// gzipWriter wraps *gzip.Writer so it satisfies pool.Resetter. gzip.Writer's
// own Reset takes an io.Writer target, which doesn't match the parameterless
// Reset() the pool needs to recycle an instance, so this adds a Reset() that
// detaches it back onto io.Discard.
type gzipWriter struct {
	*gzip.Writer
}

func (w *gzipWriter) Reset() {
	w.Writer.Reset(io.Discard)
}

var gzipWriterPool = pool.New(func() *gzipWriter {
	return &gzipWriter{gzip.NewWriter(io.Discard)}
})

func (w *responseCompressedWriter) checkContentType() {
	if w.checked {
		return
	}
	w.checked = true
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "application/json") || strings.Contains(ct, "text/html") {
		w.gw = gzipWriterPool.Get()
		w.gw.Writer.Reset(w.ResponseWriter)
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

// generate:reset
type signResponseWriter struct {
	http.ResponseWriter
	buf        bytes.Buffer
	statusCode int
}

func (w *signResponseWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

func (w *signResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func finalizeSignedResponse(w http.ResponseWriter, sw *signResponseWriter, key string) {
	signature := sign.Sign(sw.buf.Bytes(), key)
	w.Header().Set("HashSHA256", signature)
	w.WriteHeader(sw.statusCode)
	_, _ = w.Write(sw.buf.Bytes())
}

func loggingMiddleware(l *zap.Logger) func(http.Handler) http.Handler {
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

func contentTypeMiddleware(contentType string) func(handler http.Handler) http.Handler {
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

func compressMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
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
					defer func() { _ = gr.Close() }()
					r.Body = gr
				}
				if strings.Contains(aEnc, "gzip") {
					cw := &responseCompressedWriter{ResponseWriter: w}
					defer func() {
						if cw.gw != nil {
							if err := cw.gw.Close(); err != nil {
								logger.Warn("gzip writer close failed", zap.Error(err))
							}
							// Put resets cw.gw back onto io.Discard via gzipWriter.Reset,
							// detaching it from w.ResponseWriter before it returns to the pool.
							gzipWriterPool.Put(cw.gw)
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

func signMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if key == "" {
					next.ServeHTTP(w, r)
					return
				}

				sw := &signResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

				bodyBytes, err := io.ReadAll(r.Body)
				if err != nil {
					sw.WriteHeader(http.StatusBadRequest)
					finalizeSignedResponse(w, sw, key)
					return
				}
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				signature := r.Header.Get("HashSHA256")
				if signature != "" && !sign.Verify(bodyBytes, key, signature) {
					sw.WriteHeader(http.StatusBadRequest)
					finalizeSignedResponse(w, sw, key)
					return
				}

				next.ServeHTTP(sw, r)
				finalizeSignedResponse(w, sw, key)
			},
		)
	}
}
