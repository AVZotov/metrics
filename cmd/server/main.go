// Command server runs the HTTP API that receives metrics from agents,
// persists them (in memory, to a file, or to Postgres), and serves them back.
package main

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/AVZotov/metrics/internal/audit"
	"github.com/AVZotov/metrics/internal/buildinfo"
	"github.com/AVZotov/metrics/internal/config"
	"github.com/AVZotov/metrics/internal/encrypt"
	"github.com/AVZotov/metrics/internal/grpcserver"
	"github.com/AVZotov/metrics/internal/handler"
	"github.com/AVZotov/metrics/internal/repository"
	"github.com/AVZotov/metrics/internal/service"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var buildVersion string
var buildDate string
var buildCommit string

const auditShutdownTimeout = 1 * time.Second

// serveErrBuffer has one slot per server (HTTP and gRPC), so a failing
// Serve goroutine never blocks on send, even after run() stopped reading.
const serveErrBuffer = 2

func main() {
	buildinfo.Print(buildVersion, buildDate, buildCommit)
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT,
	)
	defer cancel()
	wg := sync.WaitGroup{}

	cfg, err := config.NewServerConfig()
	if err != nil {
		return err
	}
	logger, err := zap.NewDevelopment()
	if err != nil {
		return err
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil {
			logger.Error(syncErr.Error())
		}
	}()
	var privateKey *rsa.PrivateKey
	if cfg.CryptoKey != "" {
		privateKey, err = encrypt.LoadPrivateKey(cfg.CryptoKey)
		if err != nil {
			return err
		}
	}
	mStore := repository.NewMemStore()
	pStore, err := getPersistStore(cfg)
	if err != nil {
		return err
	}
	repo, err := initRepo(ctx, cfg, mStore, pStore, &wg)
	if err != nil {
		return err
	}
	auditNotifier := audit.NewNotifier(&cfg.Audit, logger)
	s := service.NewMetricsService(repo, auditNotifier)
	h := handler.New(s, logger)
	mux := handler.NewRouter(
		h, logger, handler.RouterOptions{
			Key:           cfg.Key,
			EnablePprof:   cfg.EnablePprof,
			PrivateKey:    privateKey,
			TrustedSubnet: cfg.TrustedSubnet,
		},
	)
	serveErr := make(chan error, serveErrBuffer)
	var grpcServer *grpc.Server
	if cfg.GRPCAddress != "" {
		grpcServer, err = startGRPC(cfg, s, logger, serveErr)
		if err != nil {
			return err
		}
	}
	server := &http.Server{
		Addr:    cfg.String(),
		Handler: mux,
	}
	startHTTP(server, serveErr)

	return waitAndShutdown(ctx, cancel, serveErr, shutdownTargets{
		httpServer:  server,
		grpcServer:  grpcServer,
		wg:          &wg,
		repo:        repo,
		audit:       auditNotifier,
		gracePeriod: time.Duration(cfg.ShutdownGracePeriod) * time.Second,
	}, logger)
}

// shutdownTargets is everything waitAndShutdown stops, in the order it
// stops them.
type shutdownTargets struct {
	httpServer *http.Server
	// grpcServer is nil when gRPC is disabled.
	grpcServer *grpc.Server
	// wg tracks background workers that exit when run's ctx is cancelled.
	wg   *sync.WaitGroup
	repo interface {
		Dump() error
		Close() error
	}
	audit interface {
		Shutdown(ctx context.Context) error
	}
	gracePeriod time.Duration
}

// waitAndShutdown blocks until ctx is done or a server fails, then stops
// everything in t: HTTP, gRPC, background workers, the final Dump, the
// repository and the audit notifier. cancel stops the workers tracked by
// t.wg, which is needed when a server failure, not ctx, triggered the
// shutdown. Returns the server failure, if any, joined with every error
// from the shutdown steps.
func waitAndShutdown(
	ctx context.Context, cancel context.CancelFunc, serveErr <-chan error,
	t shutdownTargets, logger *zap.Logger,
) error {
	var errs error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		logger.Error("server failed, shutting down", zap.Error(err))
		errs = err
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), t.gracePeriod)
	defer shutdownCancel()

	logger.Info("shutting down server...")
	if err := t.httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error(err.Error())
		errs = errors.Join(errs, err)
	}
	if t.grpcServer != nil {
		stopGRPC(shutdownCtx, t.grpcServer, logger)
	}
	t.wg.Wait()
	if err := t.repo.Dump(); err != nil {
		logger.Error(err.Error())
		errs = errors.Join(errs, err)
	}
	if err := t.repo.Close(); err != nil {
		logger.Error(err.Error())
		errs = errors.Join(errs, err)
	}

	auditCtx, auditCancel := context.WithTimeout(context.Background(), auditShutdownTimeout)
	defer auditCancel()

	if err := t.audit.Shutdown(auditCtx); err != nil {
		logger.Error(err.Error())
		errs = errors.Join(errs, err)
	}

	return errs
}

// startHTTP serves server in the background. A failure other than a
// graceful Shutdown is sent to serveErr.
func startHTTP(server *http.Server, serveErr chan<- error) {
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("serve HTTP: %w", err)
		}
	}()
}

func getPersistStore(cfg *config.ServerConfig) (repository.PersistRepository, error) {
	switch {
	case cfg.DSNSet:
		ctx, cancel := context.WithTimeout(context.Background(), cfg.DB.ConnectTimeout)
		defer cancel()
		if err := repository.RunMigrations(ctx, cfg.DSN); err != nil {
			return nil, err
		}
		return repository.NewDBStore(ctx, cfg.DSN, &cfg.DB)
	case cfg.FileStoragePath != "":
		return repository.NewFileStore(filepath.Base(cfg.FileStoragePath), filepath.Dir(cfg.FileStoragePath))
	default:
		return repository.NewNoopStore(), nil
	}
}

func initRepo(
	ctx context.Context,
	cfg *config.ServerConfig,
	mStore repository.Repository,
	pStore repository.PersistRepository,
	wg *sync.WaitGroup,
) (*repository.Store, error) {
	repo := repository.NewStore(mStore, pStore, cfg.StoreInterval == 0)
	if cfg.Restore {
		if err := repo.Restore(); err != nil {
			return nil, err
		}
	}
	if cfg.StoreInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Duration(cfg.StoreInterval) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := repo.Dump(); err != nil {
						log.Println(err)
					}
				}
			}
		}()
	}

	return repo, nil
}

// startGRPC listens on cfg.GRPCAddress and serves the Metrics service over
// gRPC with TLS in the background. Returns an error if the TLS certificate
// can't be loaded or the address can't be bound.
// A Serve failure other than a graceful stop is sent to serveErr.
func startGRPC(
	cfg *config.ServerConfig, svc *service.MetricsService, logger *zap.Logger, serveErr chan<- error,
) (*grpc.Server, error) {
	if cfg.Key != "" || cfg.CryptoKey != "" {
		logger.Warn(
			"gRPC traffic is encrypted with TLS, but signature verification (-k) and " +
				"body decryption (-crypto-key) apply to HTTP only; gRPC metrics are accepted without them",
		)
	}
	creds, err := credentials.NewServerTLSFromFile(cfg.GRPCCert, cfg.GRPCKey)
	if err != nil {
		return nil, fmt.Errorf("load gRPC TLS certificate: %w", err)
	}
	srv, err := grpcserver.NewGRPCServer(svc, cfg.TrustedSubnet, creds, logger)
	if err != nil {
		return nil, err
	}
	lis, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		srv.Stop()
		return nil, fmt.Errorf("listen gRPC on %s: %w", cfg.GRPCAddress, err)
	}
	go func() {
		// Serve returns ErrServerStopped if shutdown stops the server
		// before this goroutine reaches Serve.
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErr <- fmt.Errorf("serve gRPC: %w", err)
		}
	}()
	logger.Info("gRPC server listening", zap.String("address", cfg.GRPCAddress))
	return srv, nil
}

// stopGRPC stops srv gracefully. If ctx expires first, it calls Stop, which
// closes client connections and cancels in-flight calls' contexts, then
// still waits for running handlers to return: grpc-go's GracefulStop
// always does. A handler that ignores its ctx can therefore delay shutdown
// past ctx's deadline
func stopGRPC(ctx context.Context, srv *grpc.Server, logger *zap.Logger) {
	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		logger.Warn("gRPC graceful stop timed out, forcing stop", zap.Error(ctx.Err()))
		srv.Stop()
		<-done
	}
}
