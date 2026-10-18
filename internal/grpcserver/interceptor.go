package grpcserver

import (
	"context"
	"net/netip"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/AVZotov/metrics/internal/trustedsubnet"
)

// RecoveryInterceptor turns a panic in any later interceptor or handler into
// codes.Internal and logs it with the stack, so one bad call can't crash the
// server. Install it first in the chain so it wraps everything after it.
func RecoveryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(
					"panic in gRPC handler",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r),
					zap.Stack("stack"),
				)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// TrustedSubnetInterceptor rejects calls whose x-real-ip metadata is missing,
// unparsable or outside subnet with codes.PermissionDenied. subnet must be
// valid. NewGRPCServer installs the interceptor only when a subnet is
// configured.
func TrustedSubnetInterceptor(subnet netip.Prefix, logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		var rawIP string
		if vals := metadata.ValueFromIncomingContext(ctx, "x-real-ip"); len(vals) > 0 {
			rawIP = vals[0]
		}
		if err := trustedsubnet.Check(subnet, rawIP); err != nil {
			logger.Warn("request rejected by trusted subnet", zap.String("ip", rawIP), zap.Error(err))
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
		return handler(ctx, req)
	}
}
