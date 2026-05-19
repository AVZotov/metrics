package handler

import "net/http"

func NewRouter(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	register(mux, h)
	return mux
}

func register(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc(`POST /update/`, h.update)
	mux.HandleFunc(`POST /update`, h.badRequest)
}
