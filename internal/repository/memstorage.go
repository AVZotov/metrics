package repository

import (
	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

var _ Repository = (*MemStorage)(nil)

type MemStorage struct {
	gauge   map[string]float64
	counter map[string]int64
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauge:   make(map[string]float64),
		counter: make(map[string]int64),
	}
}

func (m *MemStorage) Save(metrics *models.Metrics) error {
	switch metrics.MType {
	case models.Counter:
		if metrics.Delta == nil {
			return errors.ErrNilDelta
		}
		m.counter[metrics.ID] += *metrics.Delta
	case models.Gauge:
		if metrics.Value == nil {
			return errors.ErrNilValue
		}
		m.gauge[metrics.ID] = *metrics.Value
	default:
		return errors.ErrUnknownMetricType
	}
	return nil
}
