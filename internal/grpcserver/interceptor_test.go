package grpcserver

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestTrustedSubnetInterceptor(t *testing.T) {
	trusted := netip.MustParsePrefix("192.168.0.0/24")

	tests := []struct {
		name          string
		trustedSubnet netip.Prefix
		// md is the incoming metadata; nil means the context carries none.
		md         metadata.MD
		wantCode   codes.Code
		wantCalled bool
	}{
		{
			name:          "trusted subnet not specified on server call should pass",
			trustedSubnet: netip.Prefix{},
			md:            metadata.Pairs("x-real-ip", "10.0.0.1"),
			wantCode:      codes.OK,
			wantCalled:    true,
		},
		{
			name:          "call from trusted subnet should pass",
			trustedSubnet: trusted,
			md:            metadata.Pairs("x-real-ip", "192.168.0.5"),
			wantCode:      codes.OK,
			wantCalled:    true,
		},
		{
			name:          "no metadata in context should fail",
			trustedSubnet: trusted,
			md:            nil,
			wantCode:      codes.PermissionDenied,
		},
		{
			name:          "metadata without x-real-ip should fail",
			trustedSubnet: trusted,
			md:            metadata.Pairs("user-agent", "agent"),
			wantCode:      codes.PermissionDenied,
		},
		{
			name:          "empty x-real-ip should fail",
			trustedSubnet: trusted,
			md:            metadata.Pairs("x-real-ip", ""),
			wantCode:      codes.PermissionDenied,
		},
		{
			name:          "garbage x-real-ip should fail",
			trustedSubnet: trusted,
			md:            metadata.Pairs("x-real-ip", "garbage"),
			wantCode:      codes.PermissionDenied,
		},
		{
			name:          "ip not in trusted subnet should fail",
			trustedSubnet: trusted,
			md:            metadata.Pairs("x-real-ip", "192.168.1.1"),
			wantCode:      codes.PermissionDenied,
		},
		{
			name:          "ip wrapped to IPv6 and in trusted subnet range should pass",
			trustedSubnet: trusted,
			md:            metadata.Pairs("x-real-ip", "::ffff:192.168.0.5"),
			wantCode:      codes.OK,
			wantCalled:    true,
		},
	}

	logger := zap.NewNop()
	info := &grpc.UnaryServerInfo{FullMethod: "/metrics.Metrics/UpdateMetrics"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.md != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.md)
			}
			called := false
			handler := func(context.Context, any) (any, error) {
				called = true
				return "ok", nil
			}

			interceptor := TrustedSubnetInterceptor(tt.trustedSubnet, logger)
			var err error
			require.NotPanics(t, func() {
				_, err = interceptor(ctx, nil, info, handler)
			})

			assert.Equal(t, tt.wantCode, status.Code(err))
			assert.Equal(t, tt.wantCalled, called)
		})
	}
}
