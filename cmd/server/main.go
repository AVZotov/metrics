package main

import (
	"log"
	"net/http"
	
	"github.com/AVZotov/metrics/internal/handler"
	"github.com/AVZotov/metrics/internal/repository"
	"github.com/AVZotov/metrics/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	r := repository.NewMemStorage()
	s := service.NewMetricsService(r)
	h := handler.New(s)
	mux := handler.NewRouter(h)
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	return server.ListenAndServe()
}
