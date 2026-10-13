package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	pb "github.com/AVZotov/metrics/internal/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// blockingMetricsServer blocks its single UpdateMetrics call until release
// is closed. With honorCtx it also returns when the call's context is
// cancelled; without it, it models a handler stuck in storage that ignores
// cancellation, like the real grpcserver.Server (the service API takes no
// context).
type blockingMetricsServer struct {
	pb.UnimplementedMetricsServer
	honorCtx bool
	started  chan struct{}
	release  chan struct{}
	done     chan struct{}
}

func newBlockingMetricsServer(honorCtx bool) *blockingMetricsServer {
	return &blockingMetricsServer{
		honorCtx: honorCtx,
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (s *blockingMetricsServer) UpdateMetrics(
	ctx context.Context, _ *pb.UpdateMetricsRequest,
) (*pb.UpdateMetricsResponse, error) {
	defer close(s.done)
	close(s.started)
	if !s.honorCtx {
		<-s.release
		return &pb.UpdateMetricsResponse{}, nil
	}
	select {
	case <-s.release:
		return &pb.UpdateMetricsResponse{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// startGRPC serves svc on a random local port and returns the server, its
// address, and a channel that receives Serve's result.
func startGRPC(t *testing.T, svc pb.MetricsServer) (*grpc.Server, string, <-chan error) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	pb.RegisterMetricsServer(srv, svc)
	serveErr := make(chan error, 1)
	go func() {
		// A stop that wins the race with Serve makes it return
		// ErrServerStopped, which run() treats as a clean exit too.
		err := srv.Serve(lis)
		if errors.Is(err, grpc.ErrServerStopped) {
			err = nil
		}
		serveErr <- err
	}()
	return srv, lis.Addr().String(), serveErr
}

func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.WarnLevel)
	return zap.New(core), logs
}

// startHungCall starts an UpdateMetrics call against svc at addr and waits
// until it's in progress on the server. The returned channel receives the
// client-side result.
func startHungCall(t *testing.T, svc *blockingMetricsServer, addr string) <-chan error {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	callErr := make(chan error, 1)
	go func() {
		_, err := pb.NewMetricsClient(conn).UpdateMetrics(context.Background(), &pb.UpdateMetricsRequest{})
		callErr <- err
	}()

	select {
	case <-svc.started:
	case <-time.After(5 * time.Second):
		t.Fatal("call never reached the server")
	}
	return callErr
}

// waitFor returns the next value from ch, failing the test after 5s.
func waitFor[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: still blocked after 5s", what)
		var zero T
		return zero
	}
}

// TestStopGRPC_NoInFlightCalls_StopsGracefully verifies that with no calls
// in progress stopGRPC returns right away without forcing a hard stop.
func TestStopGRPC_NoInFlightCalls_StopsGracefully(t *testing.T) {
	srv, _, serveErr := startGRPC(t, newBlockingMetricsServer(true))
	logger, logs := newObservedLogger()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	stopGRPC(ctx, srv, logger)

	assert.Less(t, time.Since(start), time.Second)
	assert.Zero(t, logs.Len(), "graceful stop must not log a forced stop")
	require.NoError(t, waitFor(t, serveErr, "Serve"), "Serve must exit cleanly after a stop")
}

// TestStopGRPC_HungCall_ForcesStopAfterTimeout verifies that a call that
// doesn't finish makes stopGRPC fall back to Stop once ctx expires: it
// returns after about the timeout, the client call fails, and no goroutine
// is left blocked. The handler returns as soon as Stop cancels its context.
func TestStopGRPC_HungCall_ForcesStopAfterTimeout(t *testing.T) {
	svc := newBlockingMetricsServer(true)
	srv, addr, serveErr := startGRPC(t, svc)
	logger, logs := newObservedLogger()
	callErr := startHungCall(t, svc, addr)

	const timeout = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	stopGRPC(ctx, srv, logger)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, timeout)
	assert.Less(t, elapsed, 2*time.Second)
	assert.Equal(t, 1, logs.FilterMessage("gRPC graceful stop timed out, forcing stop").Len())
	assert.Error(t, waitFor(t, callErr, "client call"), "client call must fail after a forced stop")
	require.NoError(t, waitFor(t, serveErr, "Serve"), "Serve must exit cleanly after a stop")
	waitFor(t, svc.done, "handler")
}

// TestStopGRPC_HandlerIgnoringCtx_KnownWaitsPastTimeout documents a known
// limitation: GracefulStop always waits for running handlers, even after
// Stop, so stopGRPC can't return until a handler that ignores its
// cancelled context finishes on its own, however long after ctx expires.
// The client is still cut off at the timeout.
func TestStopGRPC_HandlerIgnoringCtx_KnownWaitsPastTimeout(t *testing.T) {
	svc := newBlockingMetricsServer(false)
	srv, addr, serveErr := startGRPC(t, svc)
	logger, _ := newObservedLogger()
	callErr := startHungCall(t, svc, addr)

	const (
		timeout      = 100 * time.Millisecond
		handlerDelay = 600 * time.Millisecond
	)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	releaseTimer := time.AfterFunc(handlerDelay, func() { close(svc.release) })
	defer releaseTimer.Stop()

	stopped := make(chan struct{})
	start := time.Now()
	go func() {
		stopGRPC(ctx, srv, logger)
		close(stopped)
	}()

	assert.Error(t, waitFor(t, callErr, "client call"), "client call must fail after a forced stop")
	assert.Less(t, time.Since(start), handlerDelay, "client must be cut off before the handler finishes")

	waitFor(t, stopped, "stopGRPC")
	assert.GreaterOrEqual(t, time.Since(start), handlerDelay, "stopGRPC returned before the handler finished")
	require.NoError(t, waitFor(t, serveErr, "Serve"))
	waitFor(t, svc.done, "handler")
}
