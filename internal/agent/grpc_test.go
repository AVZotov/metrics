package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	pb "github.com/AVZotov/metrics/internal/proto"
)

// mockMetricsClient records each UpdateMetrics call and returns err.
type mockMetricsClient struct {
	calls int
	req   *pb.UpdateMetricsRequest
	md    metadata.MD
	err   error
}

func (m *mockMetricsClient) UpdateMetrics(
	ctx context.Context, in *pb.UpdateMetricsRequest, _ ...grpc.CallOption,
) (*pb.UpdateMetricsResponse, error) {
	m.calls++
	m.req = in
	m.md, _ = metadata.FromOutgoingContext(ctx)
	if m.err != nil {
		return nil, m.err
	}
	return &pb.UpdateMetricsResponse{}, nil
}

func newGRPCAgent(t *testing.T, client pb.MetricsClient, realIP netip.Addr) *Agent {
	t.Helper()
	a, err := NewAgent(&http.Client{}, "", AgentOptions{GRPCClient: client, RealIP: realIP})
	require.NoError(t, err)
	return a
}

func TestToProto(t *testing.T) {
	value, delta := 1.5, int64(7)

	tests := []struct {
		name    string
		in      models.Metrics
		want    *pb.Metric
		wantErr error
	}{
		{
			name: "gauge",
			in:   models.Metrics{ID: "Alloc", MType: models.Gauge, Value: &value},
			want: pb.Metric_builder{Id: "Alloc", Type: pb.Metric_GAUGE, Value: value}.Build(),
		},
		{
			name: "counter",
			in:   models.Metrics{ID: "PollCount", MType: models.Counter, Delta: &delta},
			want: pb.Metric_builder{Id: "PollCount", Type: pb.Metric_COUNTER, Delta: delta}.Build(),
		},
		{
			name:    "gauge with nil value",
			in:      models.Metrics{ID: "Alloc", MType: models.Gauge, Delta: &delta},
			wantErr: apperrors.ErrEmptyMetricValue,
		},
		{
			name:    "counter with nil delta",
			in:      models.Metrics{ID: "PollCount", MType: models.Counter, Value: &value},
			wantErr: apperrors.ErrEmptyMetricValue,
		},
		{
			name:    "unknown type",
			in:      models.Metrics{ID: "x", MType: "histogram", Value: &value},
			wantErr: apperrors.ErrUnknownMetricType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toProto(tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want.GetId(), got.GetId())
			assert.Equal(t, tt.want.GetType(), got.GetType())
			assert.Equal(t, tt.want.GetValue(), got.GetValue())
			assert.Equal(t, tt.want.GetDelta(), got.GetDelta())
		})
	}
}

func TestSendMetricsGRPC_Request(t *testing.T) {
	value, delta := 1.5, int64(7)
	metrics := []models.Metrics{
		{ID: "Alloc", MType: models.Gauge, Value: &value},
		{ID: "PollCount", MType: models.Counter, Delta: &delta},
	}
	client := &mockMetricsClient{}
	a := newGRPCAgent(t, client, netip.Addr{})

	require.NoError(t, a.sendMetricsGRPC(context.Background(), metrics))

	require.Equal(t, 1, client.calls)
	got := client.req.GetMetrics()
	require.Len(t, got, 2)
	assert.Equal(t, "Alloc", got[0].GetId())
	assert.Equal(t, pb.Metric_GAUGE, got[0].GetType())
	assert.Equal(t, value, got[0].GetValue())
	assert.Equal(t, "PollCount", got[1].GetId())
	assert.Equal(t, pb.Metric_COUNTER, got[1].GetType())
	assert.Equal(t, delta, got[1].GetDelta())
}

func TestSendMetricsGRPC_RealIPMetadata(t *testing.T) {
	value := 1.5
	metrics := []models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &value}}

	tests := []struct {
		name   string
		realIP netip.Addr
		want   []string
	}{
		{name: "valid real IP is sent", realIP: netip.MustParseAddr("192.168.1.10"), want: []string{"192.168.1.10"}},
		{name: "zero real IP is omitted", realIP: netip.Addr{}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockMetricsClient{}
			a := newGRPCAgent(t, client, tt.realIP)

			require.NoError(t, a.sendMetricsGRPC(context.Background(), metrics))

			assert.Equal(t, tt.want, client.md.Get("x-real-ip"))
		})
	}
}

func TestSendMetricsGRPC_ConversionErrorSkipsCall(t *testing.T) {
	client := &mockMetricsClient{}
	a := newGRPCAgent(t, client, netip.Addr{})

	err := a.sendMetricsGRPC(context.Background(), []models.Metrics{{ID: "Alloc", MType: models.Gauge}})

	require.ErrorIs(t, err, apperrors.ErrEmptyMetricValue)
	assert.Zero(t, client.calls)
}

func TestSendMetricsGRPC_ClientErrorIsWrapped(t *testing.T) {
	value := 1.5
	clientErr := status.Error(codes.Unavailable, "connection refused")
	client := &mockMetricsClient{err: clientErr}
	a := newGRPCAgent(t, client, netip.Addr{})

	err := a.sendMetricsGRPC(context.Background(), []models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &value}})

	require.Error(t, err)
	assert.ErrorIs(t, err, clientErr)
	assert.NotEqual(t, clientErr, err, "error should be wrapped with context")
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestIsRetriable_GRPC(t *testing.T) {
	tests := []struct {
		code codes.Code
		want bool
	}{
		{codes.Unavailable, true},
		{codes.Internal, true},
		{codes.InvalidArgument, false},
		{codes.PermissionDenied, false},
		{codes.Canceled, false},
		{codes.DeadlineExceeded, false},
		{codes.Unimplemented, false},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			// Wrapped the same way sendMetricsGRPC wraps client errors.
			err := fmt.Errorf("grpc update metrics: %w", status.Error(tt.code, "x"))
			assert.Equal(t, tt.want, isRetriable(err))
		})
	}

	t.Run("plain error is not retriable", func(t *testing.T) {
		assert.False(t, isRetriable(errors.New("boom")))
	})
}

func TestSendWithRetry_UsesGRPCAndDoesNotRetryPermanentErrors(t *testing.T) {
	value := 1.5
	metrics := []models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &value}}
	client := &mockMetricsClient{err: status.Error(codes.PermissionDenied, "permission denied")}
	a := newGRPCAgent(t, client, netip.Addr{})

	err := a.SendWithRetry(context.Background(), metrics)

	retryErr, ok := errors.AsType[*apperrors.RetryError](err)
	require.True(t, ok, "expected *RetryError, got %v", err)
	assert.False(t, retryErr.Succeeded)
	assert.Len(t, retryErr.Attempts, 1)
	assert.Equal(t, 1, client.calls)
}
