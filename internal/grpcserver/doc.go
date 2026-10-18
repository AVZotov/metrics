// Package grpcserver implements the gRPC Metrics service: it converts
// incoming protobuf metrics into the service model and stores them via
// the metrics service, mapping errors to gRPC status codes.
package grpcserver
