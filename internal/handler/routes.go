package handler

import (
	"github.com/go-chi/chi/v5"
)

func NewRouter(h *Handler) *chi.Mux {
	mux := chi.NewMux()
	register(mux, h)
	return mux
}

func register(mux *chi.Mux, h *Handler) {
	mux.Post("/update/{type}/{name}/{value}", h.update)
	mux.Post("/update", h.badRequest)
	mux.Get("/value/{type}/{name}", h.getValue)
	mux.Get("/", h.getAll)
}
