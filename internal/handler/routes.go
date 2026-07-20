package handler

import (
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func NewRouter(h *Handler, logger *zap.Logger, key string) *chi.Mux {
	mux := chi.NewMux()
	mux.Use(LoggingMiddleware(logger))
	register(mux, h, key)
	return mux
}

func register(mux *chi.Mux, h *Handler, key string) {
	mux.Get("/", h.getAll)
	mux.Get("/ping", h.ping)

	mux.Group(func(mux chi.Router) {
		mux.Use(SignMiddleware(key))
		mux.Use(CompressMiddleware())
		mux.Post("/update/{type}/{name}/{value}", h.update)
		mux.Get("/value/{type}/{name}", h.getValue)
	})

	mux.Group(
		func(mux chi.Router) {
			mux.Use(SignMiddleware(key))
			mux.Use(CompressMiddleware())
			mux.Use(ContentTypeMiddleware("application/json"))
			mux.Post("/update", h.updateJSON)
			mux.Post("/update/", h.updateJSON)
			mux.Post("/value", h.valueJSON)
			mux.Post("/value/", h.valueJSON)
			mux.Post("/updates", h.updatesJSON)
			mux.Post("/updates/", h.updatesJSON)
		},
	)
}
