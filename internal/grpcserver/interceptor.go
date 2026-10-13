package grpcserver

import (
	"context"
	"net/netip"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TrustedSubnetInterceptor rejects calls whose x-real-ip metadata is missing,
// unparsable or outside subnet with codes.PermissionDenied. The zero subnet
// (IsValid() == false) disables the check and lets every call pass through.
func TrustedSubnetInterceptor(subnet netip.Prefix, logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !subnet.IsValid() {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			logger.Warn("metadata not found in incoming context")
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
		vals := md.Get("x-real-ip")
		if len(vals) == 0 || vals[0] == "" {
			logger.Warn("x-real-ip not found in metadata")
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
		rawIP := vals[0]
		ip, err := netip.ParseAddr(rawIP)
		if err != nil {
			logger.Warn("failed to parse ip", zap.String("ip", rawIP), zap.Error(err))
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
		cleanIP := ip.Unmap()
		if !subnet.Contains(cleanIP) {
			logger.Warn("ip not in subnet", zap.String("ip", cleanIP.String()))
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
		return handler(ctx, req)
	}
}
