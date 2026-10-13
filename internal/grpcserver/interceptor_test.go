package grpcserver

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	models "github.com/AVZotov/metrics/internal/model"
	pb "github.com/AVZotov/metrics/internal/proto"
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

// panicOnceUpdater panics on its first UpdateMetrics call and succeeds after.
type panicOnceUpdater struct {
	calls atomic.Int32
}

func (p *panicOnceUpdater) UpdateMetrics([]models.Metrics, string) error {
	if p.calls.Add(1) == 1 {
		panic("boom")
	}
	return nil
}

func TestRecoveryInterceptor_HandlerPanic(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			RecoveryInterceptor(logger),
			TrustedSubnetInterceptor(netip.Prefix{}, logger),
		),
	)
	pb.RegisterMetricsServer(srv, NewServer(&panicOnceUpdater{}, logger))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := pb.NewMetricsClient(conn)

	req := &pb.UpdateMetricsRequest{
		Metrics: []*pb.Metric{{Id: "Alloc", Type: pb.Metric_GAUGE, Value: 1}},
	}

	_, err = client.UpdateMetrics(context.Background(), req)
	assert.Equal(t, codes.Internal, status.Code(err))

	_, err = client.UpdateMetrics(context.Background(), req)
	require.NoError(t, err, "server must keep serving after a recovered panic")

	entries := logs.FilterMessage("panic in gRPC handler").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, pb.Metrics_UpdateMetrics_FullMethodName, fields["method"])
	assert.Equal(t, "boom", fields["panic"])
	assert.NotEmpty(t, fields["stack"])
}
