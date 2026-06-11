package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	
	e "github.com/AVZotov/metrics/internal/errors"
	"github.com/AVZotov/metrics/internal/handler/templates"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/AVZotov/metrics/internal/model/dto"
	"github.com/AVZotov/metrics/internal/service"
	"github.com/go-chi/chi/v5"
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
	mType := chi.URLParam(r, "type")
	mName := chi.URLParam(r, "name")
	mValue := chi.URLParam(r, "value")
	if err := h.service.UpdateMetric(mType, mName, mValue); err != nil {
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

func (h *Handler) getValue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	mType := chi.URLParam(r, "type")
	mName := chi.URLParam(r, "name")
	m, err := h.service.GetMetric(mName, mType)
	if err != nil {
		if errors.Is(err, e.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch mType {
	case models.Counter:
		if m.Delta == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		str := strconv.FormatInt(*m.Delta, 10)
		_, err = w.Write([]byte(str))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	case models.Gauge:
		if m.Value == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		str := strconv.FormatFloat(*m.Value, 'f', -1, 64)
		_, err = w.Write([]byte(str))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func (h *Handler) getAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	m, err := h.service.GetMetrics()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	dtos := dto.GetMetricsDTOs(m)
	err = templates.MetricsPage(dtos).Render(r.Context(), w)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (h *Handler) updateJSON(w http.ResponseWriter, r *http.Request) {
	m := new(models.Metrics)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch m.MType {
	case models.Counter:
		if m.Delta == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := h.service.UpdateMetric(m.MType, m.ID, strconv.FormatInt(*m.Delta, 10)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	case models.Gauge:
		if m.Value == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := h.service.UpdateMetric(m.MType, m.ID, strconv.FormatFloat(*m.Value, 'f', -1, 64)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}
