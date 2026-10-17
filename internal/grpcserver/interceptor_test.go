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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	pb "github.com/AVZotov/metrics/internal/proto"
	"github.com/AVZotov/metrics/internal/testcert"
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

	client := serveBufconn(t, &panicOnceUpdater{}, netip.Prefix{}, logger)

	req := newGaugeRequest()

	_, err := client.UpdateMetrics(context.Background(), req)
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

// serveBufconn builds the production server via NewGRPCServer with a fresh
// test TLS certificate, serves it over an in-memory listener and returns a
// TLS client that trusts that certificate. Both are stopped when the test
// ends.
func serveBufconn(t *testing.T, svc metricsUpdater, subnet netip.Prefix, logger *zap.Logger) pb.MetricsClient {
	t.Helper()
	certPath, keyPath := testcert.Files(t)
	serverCreds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	require.NoError(t, err)
	// The bufconn target has no real host name, so the client checks the
	// certificate against "localhost", one of its SANs.
	clientCreds, err := credentials.NewClientTLSFromFile(certPath, "localhost")
	require.NoError(t, err)

	srv, err := NewGRPCServer(svc, subnet, serverCreds, logger)
	require.NoError(t, err)
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(clientCreds),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewMetricsClient(conn)
}

// TestNewGRPCServer_TrustedSubnet checks that the subnet interceptor is
// installed only when a subnet is configured.
func TestNewGRPCServer_TrustedSubnet(t *testing.T) {
	tests := []struct {
		name     string
		subnet   netip.Prefix
		realIP   string
		wantCode codes.Code
	}{
		{name: "no subnet configured, call without x-real-ip passes", subnet: netip.Prefix{}, wantCode: codes.OK},
		{name: "no subnet configured, any x-real-ip passes", subnet: netip.Prefix{}, realIP: "10.0.0.1", wantCode: codes.OK},
		{name: "subnet configured, call without x-real-ip is denied", subnet: netip.MustParsePrefix("192.168.0.0/24"), wantCode: codes.PermissionDenied},
		{name: "subnet configured, untrusted x-real-ip is denied", subnet: netip.MustParsePrefix("192.168.0.0/24"), realIP: "10.0.0.1", wantCode: codes.PermissionDenied},
		{name: "subnet configured, trusted x-real-ip passes", subnet: netip.MustParsePrefix("192.168.0.0/24"), realIP: "192.168.0.5", wantCode: codes.OK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockUpdater{}
			client := serveBufconn(t, svc, tt.subnet, zap.NewNop())

			ctx := context.Background()
			if tt.realIP != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, "x-real-ip", tt.realIP)
			}
			req := newGaugeRequest()
			_, err := client.UpdateMetrics(ctx, req)

			assert.Equal(t, tt.wantCode, status.Code(err))
			assert.Equal(t, tt.wantCode == codes.OK, svc.called)
		})
	}
}

// newGaugeRequest returns a request carrying a single gauge metric.
func newGaugeRequest() *pb.UpdateMetricsRequest {
	return pb.UpdateMetricsRequest_builder{
		Metrics: []*pb.Metric{
			pb.Metric_builder{Id: "Alloc", Type: pb.Metric_GAUGE, Value: 1}.Build(),
		},
	}.Build()
}

// TestNewGRPCServer_RequiresTLS checks that the server refuses to be built
// with credentials that would make it serve plaintext.
func TestNewGRPCServer_RequiresTLS(t *testing.T) {
	certPath, keyPath := testcert.Files(t)
	tlsCreds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	require.NoError(t, err)

	tests := []struct {
		name    string
		creds   credentials.TransportCredentials
		wantErr bool
	}{
		{name: "nil credentials", creds: nil, wantErr: true},
		// The only place insecure credentials may appear: proving they're rejected.
		{name: "insecure credentials", creds: insecure.NewCredentials(), wantErr: true},
		{name: "TLS credentials", creds: tlsCreds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, err := NewGRPCServer(&mockUpdater{}, netip.Prefix{}, tt.creds, zap.NewNop())
			if !tt.wantErr {
				require.NoError(t, err)
				require.NotNil(t, srv)
				srv.Stop()
				return
			}
			assert.ErrorIs(t, err, apperrors.ErrGRPCTLSRequired)
			assert.Nil(t, srv)
		})
	}
}
