package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	
	"github.com/AVZotov/metrics/internal/sign"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
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
					handler := compressMiddleware(zap.NewNop())(next)
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
					handler := signMiddleware(key)(next)
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

func TestTrustedSubnetMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		trustedSubnet netip.Prefix
		realIP        string
		wantCode      int
		wantCalled    bool
	}{
		{
			name:          "request from trusted subnet should pass",
			trustedSubnet: netip.MustParsePrefix("192.168.0.0/24"),
			realIP:        "192.168.0.5",
			wantCode:      http.StatusOK,
			wantCalled:    true,
		},
		{
			name:          "trusted subnet not specified on server request should pass",
			trustedSubnet: netip.Prefix{},
			realIP:        "192.168.0.5",
			wantCode:      http.StatusOK,
			wantCalled:    true,
		},
		{
			name:          "empty header request should fail",
			trustedSubnet: netip.MustParsePrefix("192.168.0.0/24"),
			realIP:        "",
			wantCode:      http.StatusForbidden,
			wantCalled:    false,
		},
		{
			name:          "garbage header in request should fail",
			trustedSubnet: netip.MustParsePrefix("192.168.0.0/24"),
			realIP:        "garbage",
			wantCode:      http.StatusForbidden,
			wantCalled:    false,
		},
		{
			name:          "request ip not in trusted subnet request should fail",
			trustedSubnet: netip.MustParsePrefix("192.168.0.0/24"),
			realIP:        "192.168.1.1",
			wantCode:      http.StatusForbidden,
			wantCalled:    false,
		},
		{
			name:          "request ip wrapped to IPv6 and in trusted subnet range request should pass",
			trustedSubnet: netip.MustParsePrefix("192.168.0.0/24"),
			realIP:        "::ffff:192.168.0.5",
			wantCode:      http.StatusOK,
			wantCalled:    true,
		},
	}
	logger := zap.NewNop()
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				called := false
				next := http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						called = true
						w.WriteHeader(http.StatusOK)
					},
				)
				req := httptest.NewRequest(http.MethodPost, "/update", nil)
				if tt.realIP != "" {
					req.Header.Set("X-Real-IP", tt.realIP)
				}
				rec := httptest.NewRecorder()
				trustedSubnetMiddleware(tt.trustedSubnet, logger)(next).ServeHTTP(rec, req)
				assert.Equal(t, tt.wantCode, rec.Code)
				assert.Equal(t, tt.wantCalled, called)
			},
		)
	}
}
