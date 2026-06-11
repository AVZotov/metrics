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
	"go.uber.org/zap"
)

type Handler struct {
	service service.Service
	logger  *zap.Logger
}

func New(s service.Service, l *zap.Logger) *Handler {
	return &Handler{
		service: s,
		logger:  l,
	}
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	mType := chi.URLParam(r, "type")
	mName := chi.URLParam(r, "name")
	mValue := chi.URLParam(r, "value")
	if err := h.service.UpdateMetric(mType, mName, mValue); err != nil {
		if errors.Is(err, e.ErrEmptyMetricName) {
			h.logger.Info("metric not found", zap.String("name", mName))
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.logger.Error("failed to update metric", zap.Error(err))
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
			h.logger.Info("metric not found", zap.String("name", mName))
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get metric", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch mType {
	case models.Counter:
		if m.Delta == nil {
			h.logger.Error("counter delta is nil", zap.String("metric", m.ID))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		str := strconv.FormatInt(*m.Delta, 10)
		_, err = w.Write([]byte(str))
		if err != nil {
			h.logger.Error("failed to write response", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	case models.Gauge:
		if m.Value == nil {
			h.logger.Error("gauge value is nil", zap.String("metric", m.ID))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		str := strconv.FormatFloat(*m.Value, 'f', -1, 64)
		_, err = w.Write([]byte(str))
		if err != nil {
			h.logger.Error("failed to write response", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func (h *Handler) getAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	m, err := h.service.GetMetrics()
	if err != nil {
		h.logger.Error("failed to get metrics", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	dtos := dto.GetMetricsDTOs(m)
	err = templates.MetricsPage(dtos).Render(r.Context(), w)
	if err != nil {
		h.logger.Error("failed to render template", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (h *Handler) updateJSON(w http.ResponseWriter, r *http.Request) {
	m := new(models.Metrics)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		h.logger.Error("failed to decode json", zap.Error(err))
		return
	}
	switch m.MType {
	case models.Counter:
		if m.Delta == nil {
			w.WriteHeader(http.StatusBadRequest)
			h.logger.Error("counter delta is nil", zap.String("metric", m.ID))
			return
		}
		if err := h.service.UpdateMetric(m.MType, m.ID, strconv.FormatInt(*m.Delta, 10)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Error("failed to update metric", zap.Error(err))
			return
		}
	case models.Gauge:
		if m.Value == nil {
			w.WriteHeader(http.StatusBadRequest)
			h.logger.Error("gauge value is nil", zap.String("metric", m.ID))
			return
		}
		if err := h.service.UpdateMetric(m.MType, m.ID, strconv.FormatFloat(*m.Value, 'f', -1, 64)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Error("failed to update metric", zap.Error(err))
			return
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(m); err != nil {
		h.logger.Error("failed to write response", zap.Error(err))
		return
	}
}

func (h *Handler) valueJSON(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	m := new(models.Metrics)
	if err := json.NewDecoder(r.Body).Decode(m); err != nil {
		h.logger.Error("failed to decode json", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if m.ID == "" || m.MType == "" {
		w.WriteHeader(http.StatusBadRequest)
		h.logger.Error("invalid json data", zap.String("metric", m.ID), zap.String("type", m.MType))
		return
	}
	got, err := h.service.GetMetric(m.MType, m.ID)
	if err != nil {
		if errors.Is(err, e.ErrNotFound) {
			h.logger.Info("metric not found", zap.String("metric", m.ID), zap.String("type", m.MType))
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if errors.Is(err, e.ErrUnknownMetricType) {
			h.logger.Info("unknown metric type", zap.String("metric", m.ID), zap.String("type", m.MType))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.logger.Error("failed to get metric", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(got); err != nil {
		h.logger.Error("failed to write response", zap.Error(err))
		return
	}
}
