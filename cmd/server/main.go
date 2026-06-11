package main

import (
	"log"
	"net/http"
	
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
	cfg, err := config.NewServerConfig()
	if err != nil {
		return err
	}
	logger, err := zap.NewDevelopment()
	if err != nil {
		return err
	}
	defer logger.Sync()
	r := repository.NewMemStorage()
	s := service.NewMetricsService(r)
	h := handler.New(s)
	mux := handler.NewRouter(h, logger)
	server := &http.Server{
		Addr:    cfg.String(),
		Handler: mux,
	}
	return server.ListenAndServe()
}
