package handler

import (
	"errors"
	"net/http"
	
	e "github.com/AVZotov/metrics/internal/errors"
	"github.com/AVZotov/metrics/internal/service"
)

type Handler struct {
	service service.Service
}

func New(s service.Service) *Handler {
	return &Handler{
		service: s,
	}
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := h.service.UpdateMetric(parseURI(r.URL.RequestURI())); err != nil {
		if errors.Is(err, e.ErrEmptyMetricName) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

func (h *Handler) badRequest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
}
