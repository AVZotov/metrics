package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AVZotov/metrics/internal/config"
	models "github.com/AVZotov/metrics/internal/model"
	pb "github.com/AVZotov/metrics/internal/proto"
	"github.com/AVZotov/metrics/internal/repository"
	"github.com/AVZotov/metrics/internal/service"
	"github.com/AVZotov/metrics/internal/testcert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

// testGRPCTarget is how a test client reaches a server from serveTestGRPC.
type testGRPCTarget struct {
	addr string
	// caPath is the server's self-signed certificate, for the client to trust.
	caPath string
}

// serveTestGRPC serves svc over TLS, with a fresh test certificate, on a
// random local port. It returns the server, how to reach it, and a channel
// that receives Serve's result.
func serveTestGRPC(t *testing.T, svc pb.MetricsServer) (*grpc.Server, testGRPCTarget, <-chan error) {
	t.Helper()
	certPath, keyPath := testcert.Files(t)
	creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	require.NoError(t, err)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer(grpc.Creds(creds))
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
	return srv, testGRPCTarget{addr: lis.Addr().String(), caPath: certPath}, serveErr
}

func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.WarnLevel)
	return zap.New(core), logs
}

// startHungCall starts an UpdateMetrics call against svc at target and
// waits until it's in progress on the server. The returned channel receives
// the client-side result.
func startHungCall(t *testing.T, svc *blockingMetricsServer, target testGRPCTarget) <-chan error {
	t.Helper()
	creds, err := credentials.NewClientTLSFromFile(target.caPath, "")
	require.NoError(t, err)
	conn, err := grpc.NewClient(target.addr, grpc.WithTransportCredentials(creds))
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
	srv, _, serveErr := serveTestGRPC(t, newBlockingMetricsServer(true))
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
	srv, target, serveErr := serveTestGRPC(t, svc)
	logger, logs := newObservedLogger()
	callErr := startHungCall(t, svc, target)

	const timeout = 200 * time.Millisecond
	// start is taken before the deadline is set, so elapsed can't come out
	// a few microseconds short of timeout.
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

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
//
// It checks the order of events, not wall-clock time: the forced stop and
// the client's failure happen while the handler is still blocked, and
// stopGRPC returns only after the test releases the handler.
func TestStopGRPC_HandlerIgnoringCtx_KnownWaitsPastTimeout(t *testing.T) {
	svc := newBlockingMetricsServer(false)
	srv, target, serveErr := serveTestGRPC(t, svc)
	logger, logs := newObservedLogger()
	callErr := startHungCall(t, svc, target)

	// An already expired ctx makes stopGRPC force a stop right away.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stopped := make(chan struct{})
	go func() {
		stopGRPC(ctx, srv, logger)
		close(stopped)
	}()

	// The handler is still blocked on release here, yet the client is cut off.
	assert.Error(t, waitFor(t, callErr, "client call"), "client call must fail after a forced stop")
	assert.Equal(t, 1, logs.FilterMessage("gRPC graceful stop timed out, forcing stop").Len())
	assert.Never(t, func() bool { return isClosed(stopped) }, 200*time.Millisecond, 10*time.Millisecond,
		"stopGRPC must not return while the handler is still running")

	close(svc.release)
	waitFor(t, stopped, "stopGRPC")
	assert.True(t, isClosed(svc.done), "stopGRPC returned before the handler finished")
	require.NoError(t, waitFor(t, serveErr, "Serve"))
}

// isClosed reports whether ch is closed, without blocking.
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// fakeShutdownDeps stands in for the repository and the audit notifier and
// records the shutdown steps waitAndShutdown runs, in order.
type fakeShutdownDeps struct {
	mu      sync.Mutex
	calls   []string
	dumpErr error
}

func (f *fakeShutdownDeps) record(step string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, step)
}

func (f *fakeShutdownDeps) steps() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeShutdownDeps) Dump() error {
	f.record("dump")
	return f.dumpErr
}

func (f *fakeShutdownDeps) Close() error {
	f.record("close")
	return nil
}

func (f *fakeShutdownDeps) Shutdown(context.Context) error {
	f.record("audit")
	return nil
}

// startWorker starts a background worker that, like the periodic Dump in
// initRepo, only exits when ctx is cancelled.
func startWorker(ctx context.Context, wg *sync.WaitGroup, deps *fakeShutdownDeps) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		deps.record("worker stopped")
	}()
}

// runWaitAndShutdown runs waitAndShutdown in the background and returns a
// channel with its result.
func runWaitAndShutdown(
	ctx context.Context, cancel context.CancelFunc, serveErr <-chan error, t shutdownTargets,
) <-chan error {
	done := make(chan error, 1)
	go func() {
		logger, _ := newObservedLogger()
		done <- waitAndShutdown(ctx, cancel, serveErr, t, logger)
	}()
	return done
}

// TestWaitAndShutdown_HTTPServeFailure verifies that when the HTTP server
// can't serve (its port is taken), the failure reaches run's shutdown path
// instead of exiting the process: background workers are stopped, Dump,
// Close and the audit shutdown still run, and the serve error is returned.
func TestWaitAndShutdown_HTTPServeFailure(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = busy.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	deps := &fakeShutdownDeps{}
	var wg sync.WaitGroup
	startWorker(ctx, &wg, deps)

	serveErr := make(chan error, serveErrBuffer)
	srv := &http.Server{Addr: busy.Addr().String()}
	startHTTP(srv, serveErr)

	err = waitFor(t, runWaitAndShutdown(ctx, cancel, serveErr, shutdownTargets{
		httpServer: srv, wg: &wg, repo: deps, audit: deps, gracePeriod: time.Second,
	}), "waitAndShutdown")

	require.Error(t, err)
	assert.ErrorContains(t, err, "serve HTTP")
	var opErr *net.OpError
	assert.ErrorAs(t, err, &opErr, "the listen error must be preserved")
	assert.Equal(t, []string{"worker stopped", "dump", "close", "audit"}, deps.steps())
}

// TestWaitAndShutdown_ServeFailureJoinsShutdownErrors verifies that a serve
// failure and a failing shutdown step are both returned.
func TestWaitAndShutdown_ServeFailureJoinsShutdownErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serveFailure := errors.New("serve gRPC: boom")
	dumpFailure := errors.New("dump failed")
	deps := &fakeShutdownDeps{dumpErr: dumpFailure}

	serveErr := make(chan error, serveErrBuffer)
	serveErr <- serveFailure

	err := waitFor(t, runWaitAndShutdown(ctx, cancel, serveErr, shutdownTargets{
		httpServer: &http.Server{}, wg: &sync.WaitGroup{}, repo: deps, audit: deps, gracePeriod: time.Second,
	}), "waitAndShutdown")

	assert.ErrorIs(t, err, serveFailure)
	assert.ErrorIs(t, err, dumpFailure)
	assert.Equal(t, []string{"dump", "close", "audit"}, deps.steps())
}

// TestWaitAndShutdown_SignalStopsCleanly verifies that a shutdown triggered
// by ctx returns nil and that the graceful HTTP stop is not reported as a
// serve failure.
func TestWaitAndShutdown_SignalStopsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	deps := &fakeShutdownDeps{}
	var wg sync.WaitGroup
	startWorker(ctx, &wg, deps)

	serveErr := make(chan error, serveErrBuffer)
	srv := &http.Server{Addr: "127.0.0.1:0"}
	startHTTP(srv, serveErr)

	done := runWaitAndShutdown(ctx, cancel, serveErr, shutdownTargets{
		httpServer: srv, wg: &wg, repo: deps, audit: deps, gracePeriod: time.Second,
	})
	cancel()

	assert.NoError(t, waitFor(t, done, "waitAndShutdown"))
	assert.Equal(t, []string{"worker stopped", "dump", "close", "audit"}, deps.steps())
	assert.Never(t, func() bool { return len(serveErr) > 0 }, 200*time.Millisecond, 10*time.Millisecond,
		"http.ErrServerClosed must not be sent to serveErr")
}

// TestStartGRPC_InvalidCertificate verifies that startGRPC refuses to start
// when the TLS certificate can't be loaded, instead of serving without TLS.
func TestStartGRPC_InvalidCertificate(t *testing.T) {
	certPath, _ := testcert.Files(t)
	_, otherKey := testcert.Files(t)
	garbage := filepath.Join(t.TempDir(), "garbage.pem")
	require.NoError(t, os.WriteFile(garbage, []byte("not a certificate"), 0o600))

	tests := []struct {
		name     string
		certPath string
		keyPath  string
	}{
		{name: "missing files", certPath: filepath.Join(t.TempDir(), "absent.pem"), keyPath: filepath.Join(t.TempDir(), "absent.pem")},
		{name: "not PEM", certPath: garbage, keyPath: garbage},
		{name: "key does not match certificate", certPath: certPath, keyPath: otherKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ServerConfig{GRPCAddress: "127.0.0.1:0", GRPCCert: tt.certPath, GRPCKey: tt.keyPath}
			serveErr := make(chan error, serveErrBuffer)

			srv, err := startGRPC(cfg, nil, zap.NewNop(), serveErr)

			assert.ErrorContains(t, err, "load gRPC TLS certificate")
			assert.Nil(t, srv)
		})
	}
}

// freeAddr returns a local address that was free a moment ago. startGRPC
// binds the address itself and doesn't report a ":0" port back, so the
// test has to pick one.
func freeAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())
	return addr
}

// newTestService returns a metrics service backed by memory only.
func newTestService() *service.MetricsService {
	store := repository.NewStore(repository.NewMemStore(), repository.NewNoopStore(), false)
	return service.NewMetricsService(store, nil)
}

// TestStartGRPC_ServesTLS verifies that startGRPC serves the Metrics
// service over TLS: a client that trusts the certificate stores a metric.
func TestStartGRPC_ServesTLS(t *testing.T) {
	certPath, keyPath := testcert.Files(t)
	cfg := &config.ServerConfig{GRPCAddress: freeAddr(t), GRPCCert: certPath, GRPCKey: keyPath}
	svc := newTestService()
	serveErr := make(chan error, serveErrBuffer)

	srv, err := startGRPC(cfg, svc, zap.NewNop(), serveErr)
	require.NoError(t, err)
	t.Cleanup(srv.Stop)

	creds, err := credentials.NewClientTLSFromFile(certPath, "")
	require.NoError(t, err)
	conn, err := grpc.NewClient(cfg.GRPCAddress, grpc.WithTransportCredentials(creds))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := pb.UpdateMetricsRequest_builder{
		Metrics: []*pb.Metric{pb.Metric_builder{Id: "Alloc", Type: pb.Metric_GAUGE, Value: 1.5}.Build()},
	}.Build()
	_, err = pb.NewMetricsClient(conn).UpdateMetrics(ctx, req)
	require.NoError(t, err)

	got, err := svc.GetMetric("Alloc", models.Gauge)
	require.NoError(t, err)
	require.NotNil(t, got.Value)
	assert.Equal(t, 1.5, *got.Value)
	assert.Empty(t, serveErr, "Serve must not report an error while serving")
}

// TestStartGRPC_AddressInUse verifies that startGRPC returns the listen
// error when the port is taken and doesn't hold the port afterwards.
func TestStartGRPC_AddressInUse(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := busy.Addr().String()

	certPath, keyPath := testcert.Files(t)
	cfg := &config.ServerConfig{GRPCAddress: addr, GRPCCert: certPath, GRPCKey: keyPath}
	serveErr := make(chan error, serveErrBuffer)

	srv, err := startGRPC(cfg, newTestService(), zap.NewNop(), serveErr)

	assert.ErrorContains(t, err, "listen gRPC on "+addr)
	assert.Nil(t, srv)

	// Once the test's own listener is gone, the port must be free again:
	// startGRPC left nothing bound to it.
	require.NoError(t, busy.Close())
	again, err := net.Listen("tcp", addr)
	require.NoError(t, err, "port must not stay taken after a failed startGRPC")
	require.NoError(t, again.Close())
}
