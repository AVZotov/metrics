package agent

import (
	"context"
	"fmt"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	pb "github.com/AVZotov/metrics/internal/proto"
	"google.golang.org/grpc/metadata"
)

// toProto converts a service-model metric into its protobuf form.
func toProto(m models.Metrics) (*pb.Metric, error) {
	switch m.MType {
	case models.Gauge:
		if m.Value == nil {
			return nil, apperrors.ErrEmptyMetricValue
		}
		return &pb.Metric{Id: m.ID, Type: pb.Metric_GAUGE, Value: *m.Value}, nil
	case models.Counter:
		if m.Delta == nil {
			return nil, apperrors.ErrEmptyMetricValue
		}
		return &pb.Metric{Id: m.ID, Type: pb.Metric_COUNTER, Delta: *m.Delta}, nil
	default:
		return nil, fmt.Errorf("%w: %s", apperrors.ErrUnknownMetricType, m.MType)
	}
}

// sendMetricsGRPC sends a batch of metrics to the server over gRPC.
func (a *Agent) sendMetricsGRPC(ctx context.Context, metrics []models.Metrics) error {
	protoMetrics := make([]*pb.Metric, 0, len(metrics))
	for _, mm := range metrics {
		pm, err := toProto(mm)
		if err != nil {
			return err
		}
		protoMetrics = append(protoMetrics, pm)
	}
	req := &pb.UpdateMetricsRequest{Metrics: protoMetrics}
	if a.realIP.IsValid() {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-real-ip", a.realIP.String())
	}
	_, err := a.grpcClient.UpdateMetrics(ctx, req)
	if err != nil {
		return fmt.Errorf("grpc update metrics: %w", err)
	}
	return nil
}
