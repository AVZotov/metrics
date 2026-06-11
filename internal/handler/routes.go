package handler

import (
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func NewRouter(h *Handler, logger *zap.Logger) *chi.Mux {
	mux := chi.NewMux()
	mux.Use(LoggingMiddleware(logger))
	register(mux, h)
	return mux
}

func register(mux *chi.Mux, h *Handler) {
	mux.Group(
		func(mux chi.Router) {
			mux.Use(ContentTypeMiddleware("text/plain"))
			mux.Post("/update/{type}/{name}/{value}", h.update)
		},
	)
	
	mux.Group(
		func(mux chi.Router) {
			mux.Use(ContentTypeMiddleware("application/json"))
			mux.Post("/update", h.updateJSON)
			mux.Post("/value", h.valueJSON)
		},
	)
	
	mux.Get("/value/{type}/{name}", h.getValue)
	mux.Get("/", h.getAll)
}
