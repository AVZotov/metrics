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
)

var buildVersion string
var buildDate string
var buildCommit string

const auditShutdownTimeout = 1 * time.Second

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
	var grpcServer *grpc.Server
	if cfg.GRPCAddress != "" {
		grpcServer, err = startGRPC(cfg, s, logger)
		if err != nil {
			return err
		}
	}
	server := &http.Server{
		Addr:    cfg.String(),
		Handler: mux,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(), time.Duration(cfg.ShutdownGracePeriod)*time.Second,
	)
	defer shutdownCancel()

	logger.Info("shutting down server...")
	var shutdownErr error
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error(err.Error())
		shutdownErr = errors.Join(shutdownErr, err)
	}
	if grpcServer != nil {
		stopGRPC(shutdownCtx, grpcServer, logger)
	}
	wg.Wait()
	if err := repo.Dump(); err != nil {
		logger.Error(err.Error())
		shutdownErr = errors.Join(shutdownErr, err)
	}
	if err := repo.Close(); err != nil {
		logger.Error(err.Error())
		shutdownErr = errors.Join(shutdownErr, err)
	}

	auditCtx, auditCancel := context.WithTimeout(context.Background(), auditShutdownTimeout)
	defer auditCancel()

	if err := auditNotifier.Shutdown(auditCtx); err != nil {
		logger.Error(err.Error())
		shutdownErr = errors.Join(shutdownErr, err)
	}

	return shutdownErr
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
// gRPC in the background. Returns an error if the address can't be bound.
func startGRPC(cfg *config.ServerConfig, svc *service.MetricsService, logger *zap.Logger) (*grpc.Server, error) {
	if cfg.Key != "" || cfg.CryptoKey != "" {
		logger.Warn(
			"signature verification and decryption apply to HTTP only; " +
				"the gRPC endpoint accepts unsigned plaintext metrics",
		)
	}
	lis, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("listen gRPC on %s: %w", cfg.GRPCAddress, err)
	}
	srv := grpcserver.NewGRPCServer(svc, cfg.TrustedSubnet, logger)
	go func() {
		// Serve returns ErrServerStopped if shutdown stops the server
		// before this goroutine reaches Serve.
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Fatal(err)
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
