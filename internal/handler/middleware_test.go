package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AVZotov/metrics/internal/sign"
)

func BenchmarkCompressMiddleware(b *testing.B) {
	benchmarks := []struct {
		name string
		data []byte
	}{
		{name: "100 bytes", data: bytes.Repeat([]byte("a"), 100)},
		{name: "1024 bytes", data: bytes.Repeat([]byte("a"), 1024)},
		{name: "10240 bytes", data: bytes.Repeat([]byte("a"), 10240)},
	}
	for _, bm := range benchmarks {
		b.Run(
			bm.name, func(b *testing.B) {
				{
					b.ReportAllocs()
					next := http.HandlerFunc(
						func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write(bm.data)
						},
					)
					handler := CompressMiddleware()(next)
					for b.Loop() {
						b.StopTimer()
						req := httptest.NewRequest(http.MethodGet, "/", nil)
						req.Header.Set("Accept-Encoding", "gzip")
						rec := httptest.NewRecorder()
						b.StartTimer()
						handler.ServeHTTP(rec, req)
					}
				}
			},
		)
	}
}

func BenchmarkSignMiddleware(b *testing.B) {
	const key = "super secret key"
	benchmarks := []struct {
		name string
		data []byte
	}{
		{name: "nil", data: nil},
		{name: "100 bytes", data: bytes.Repeat([]byte("a"), 100)},
		{name: "1024 bytes", data: bytes.Repeat([]byte("a"), 1024)},
		{name: "10240 bytes", data: bytes.Repeat([]byte("a"), 10240)},
	}
	for _, bm := range benchmarks {
		b.Run(
			bm.name, func(b *testing.B) {
				{
					b.ReportAllocs()
					next := http.HandlerFunc(
						func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write(bm.data)
						},
					)
					handler := SignMiddleware(key)(next)
					signature := sign.Sign(bm.data, key)
					for b.Loop() {
						b.StopTimer()
						req, err := http.NewRequest(http.MethodPost, "/", bytes.NewReader(bm.data))
						if err != nil {
							b.Fatal(err)
						}
						req.Header.Set("HashSHA256", signature)
						rec := httptest.NewRecorder()
						b.StartTimer()
						handler.ServeHTTP(rec, req)
					}
				}
			},
		)
	}
}
