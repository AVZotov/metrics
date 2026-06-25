package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	
	"github.com/AVZotov/metrics/internal/config"
	"github.com/AVZotov/metrics/internal/handler"
	"github.com/AVZotov/metrics/internal/repository"
	"github.com/AVZotov/metrics/internal/service"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
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
		if err := logger.Sync(); err != nil {
			logger.Error(err.Error())
		}
	}()
	repo, err := initRepo(ctx, cfg, &wg)
	if err != nil {
		return err
	}
	s := service.NewMetricsService(repo)
	h := handler.New(s, logger)
	mux := handler.NewRouter(h, logger)
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
	cancel()
	wg.Wait()
	if err := repo.Dump(); err != nil {
		logger.Error(err.Error())
		shutdownErr = errors.Join(shutdownErr, err)
	}
	if err := repo.Close(); err != nil {
		logger.Error(err.Error())
		shutdownErr = errors.Join(shutdownErr, err)
	}
	
	return shutdownErr
}

func initRepo(
	ctx context.Context,
	cfg *config.ServerConfig,
	wg *sync.WaitGroup,
) (*repository.Store, error) {
	memStore := repository.NewMemStore()
	dataStore, err := repository.NewDataStore(filepath.Base(cfg.FileStoragePath), filepath.Dir(cfg.FileStoragePath))
	if err != nil {
		return nil, err
	}
	repo := repository.NewStore(memStore, dataStore, cfg.StoreInterval == 0)
	if cfg.Restore {
		if err = repo.Restore(); err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
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
