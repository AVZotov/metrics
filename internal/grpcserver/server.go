package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	pb "github.com/AVZotov/metrics/internal/proto"
)

// tlsSecurityProtocol is the SecurityProtocol that grpc-go's TLS
// credentials report in their ProtocolInfo.
const tlsSecurityProtocol = "tls"

// metricsUpdater is the part of the metrics service the gRPC server needs.
type metricsUpdater interface {
	UpdateMetrics(metrics []models.Metrics, ipAddress string) error
}

// Compile-time check that *Server satisfies the generated interface.
var _ pb.MetricsServer = (*Server)(nil)

// Server implements the gRPC Metrics service on top of the metrics service.
type Server struct {
	pb.UnimplementedMetricsServer
	service metricsUpdater
	logger  *zap.Logger
}

// NewServer returns a gRPC Metrics server that stores metrics via service.
func NewServer(service metricsUpdater, logger *zap.Logger) *Server {
	return &Server{service: service, logger: logger}
}

// NewGRPCServer returns a *grpc.Server with the Metrics service registered
// on top of service. It serves over creds, which must be TLS credentials
// (for example from credentials.NewServerTLSFromFile): the server has no
// plaintext mode, so nil or non-TLS creds return apperrors.ErrGRPCTLSRequired.
// Every unary call goes through RecoveryInterceptor first. When subnet is
// valid, TrustedSubnetInterceptor(subnet) runs next, so a panic in either
// the subnet check or the handler is recovered. The zero subnet installs
// no subnet check.
func NewGRPCServer(
	service metricsUpdater, subnet netip.Prefix, creds credentials.TransportCredentials, logger *zap.Logger,
) (*grpc.Server, error) {
	if creds == nil {
		return nil, fmt.Errorf("%w: no credentials given", apperrors.ErrGRPCTLSRequired)
	}
	if protocol := creds.Info().SecurityProtocol; protocol != tlsSecurityProtocol {
		return nil, fmt.Errorf("%w: got %q credentials", apperrors.ErrGRPCTLSRequired, protocol)
	}
	interceptors := []grpc.UnaryServerInterceptor{RecoveryInterceptor(logger)}
	if subnet.IsValid() {
		interceptors = append(interceptors, TrustedSubnetInterceptor(subnet, logger))
	}
	srv := grpc.NewServer(grpc.Creds(creds), grpc.ChainUnaryInterceptor(interceptors...))
	pb.RegisterMetricsServer(srv, NewServer(service, logger))
	return srv, nil
}

// toModel converts a protobuf metric into the service model.
func toModel(m *pb.Metric) (models.Metrics, error) {
	switch m.GetType() {
	case pb.Metric_GAUGE:
		v := m.GetValue()
		return models.Metrics{ID: m.GetId(), MType: models.Gauge, Value: &v}, nil

	case pb.Metric_COUNTER:
		d := m.GetDelta()
		return models.Metrics{ID: m.GetId(), MType: models.Counter, Delta: &d}, nil

	default:
		return models.Metrics{}, fmt.Errorf("%w: %d", apperrors.ErrUnknownMetricType, m.GetType())
	}
}

// toStatus maps a service error to a gRPC status error.
func toStatus(err error) error {
	if errors.Is(err, apperrors.ErrEmptyMetricName) ||
		errors.Is(err, apperrors.ErrUnknownMetricType) ||
		errors.Is(err, apperrors.ErrEmptyMetricValue) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, "internal server error")
}

// peerIP returns the client's IP from the gRPC connection, or "" if unknown.
func (s *Server) peerIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		s.logger.Warn("no peer in context")
		return ""
	}
	ipAddress, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		s.logger.Warn(
			"invalid remote address",
			zap.String("address", p.Addr.String()),
			zap.Error(err),
		)
		return ""
	}
	return ipAddress
}

// UpdateMetrics stores a batch of metrics sent by the agent.
func (s *Server) UpdateMetrics(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	metrics := make([]models.Metrics, 0, len(req.GetMetrics()))
	for _, pm := range req.GetMetrics() {
		m, err := toModel(pm)
		if err != nil {
			// A metric that can't be converted is the client's fault.
			s.logger.Warn("failed to convert metric", zap.Error(err))
			return nil, toStatus(err)
		}
		metrics = append(metrics, m)
	}
	ip := s.peerIP(ctx)
	if err := s.service.UpdateMetrics(metrics, ip); err != nil {
		st := toStatus(err)
		// Client errors are logged as Warn so they don't drown out real
		// server failures at Error level.
		if status.Code(st) == codes.InvalidArgument {
			s.logger.Warn("failed to update metrics", zap.Error(err))
		} else {
			s.logger.Error("failed to update metrics", zap.Error(err))
		}
		return nil, st
	}
	return &pb.UpdateMetricsResponse{}, nil
}
