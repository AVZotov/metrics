package grpcserver

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	pb "github.com/AVZotov/metrics/internal/proto"
)

// mockUpdater records what UpdateMetrics received and returns err.
type mockUpdater struct {
	called  bool
	metrics []models.Metrics
	ip      string
	err     error
}

func (m *mockUpdater) UpdateMetrics(metrics []models.Metrics, ipAddress string) error {
	m.called = true
	m.metrics = metrics
	m.ip = ipAddress
	return m.err
}

func TestServer_UpdateMetrics(t *testing.T) {
	withPeer := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 54321},
	})

	gauge := &pb.Metric{Id: "Alloc", Type: pb.Metric_GAUGE, Value: 1.5}
	counter := &pb.Metric{Id: "PollCount", Type: pb.Metric_COUNTER, Delta: 7}

	value, delta := 1.5, int64(7)
	wantBatch := []models.Metrics{
		{ID: "Alloc", MType: models.Gauge, Value: &value},
		{ID: "PollCount", MType: models.Counter, Delta: &delta},
	}

	tests := []struct {
		name        string
		ctx         context.Context
		metrics     []*pb.Metric
		svcErr      error
		wantCode    codes.Code
		wantCalled  bool
		wantMetrics []models.Metrics
		wantIP      string
	}{
		{
			name:        "gauge and counter batch",
			ctx:         withPeer,
			metrics:     []*pb.Metric{gauge, counter},
			wantCode:    codes.OK,
			wantCalled:  true,
			wantMetrics: wantBatch,
			wantIP:      "10.0.0.5",
		},
		{
			name:     "unknown metric type",
			ctx:      withPeer,
			metrics:  []*pb.Metric{{Id: "x", Type: pb.Metric_MType(5)}},
			wantCode: codes.InvalidArgument,
		},
		{
			name:        "service rejects empty name",
			ctx:         withPeer,
			metrics:     []*pb.Metric{gauge},
			svcErr:      apperrors.ErrEmptyMetricName,
			wantCode:    codes.InvalidArgument,
			wantCalled:  true,
			wantMetrics: wantBatch[:1],
			wantIP:      "10.0.0.5",
		},
		{
			name:        "service internal error",
			ctx:         withPeer,
			metrics:     []*pb.Metric{gauge},
			svcErr:      errors.New("db down"),
			wantCode:    codes.Internal,
			wantCalled:  true,
			wantMetrics: wantBatch[:1],
			wantIP:      "10.0.0.5",
		},
		{
			name:        "no peer in context",
			ctx:         context.Background(),
			metrics:     []*pb.Metric{gauge},
			wantCode:    codes.OK,
			wantCalled:  true,
			wantMetrics: wantBatch[:1],
			wantIP:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockUpdater{err: tt.svcErr}
			srv := NewServer(svc, zap.NewNop())

			resp, err := srv.UpdateMetrics(tt.ctx, &pb.UpdateMetricsRequest{Metrics: tt.metrics})

			require.Equal(t, tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				assert.NotNil(t, resp)
			} else {
				assert.Nil(t, resp)
			}

			require.Equal(t, tt.wantCalled, svc.called)
			if tt.wantCalled {
				assert.Equal(t, tt.wantMetrics, svc.metrics)
				assert.Equal(t, tt.wantIP, svc.ip)
			}

			if tt.svcErr != nil && tt.wantCode == codes.Internal {
				assert.NotContains(t, status.Convert(err).Message(), tt.svcErr.Error())
			}
		})
	}
}
