package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
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
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	
	cfg, err := config.NewServerConfig()
	if err != nil {
		return err
	}
	logger, err := zap.NewDevelopment()
	if err != nil {
		return err
	}
	defer logger.Sync()
	repo, err := initRepo(cfg, done)
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
	
	<-done
	log.Println("shutting down...")
	return nil
}

func initRepo(cfg *config.ServerConfig, done chan os.Signal) (repository.Repository, error) {
	memStore := repository.NewMemStorage()
	dir, file := path.Split(cfg.FileStoragePath)
	if file == "" {
		return nil, errors.New("file name is empty")
	}
	dataStore, err := repository.NewDataStore(file, dir)
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
		go func(done chan os.Signal) {
			ticker := time.NewTicker(time.Duration(cfg.StoreInterval) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					if err := repo.Dump(); err != nil {
						log.Println(err)
					}
				}
			}
		}(done)
	}
	return repo, nil
}
