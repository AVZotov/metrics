package repository

import (
	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

var _ Repository = (*MemStorage)(nil)

type MemStorage struct {
	gauge   map[string]models.Metrics
	counter map[string]models.Metrics
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauge:   make(map[string]models.Metrics),
		counter: make(map[string]models.Metrics),
	}
}

func (m *MemStorage) Save(metrics *models.Metrics) error {
	if metrics == nil {
		return errors.ErrNilMetric
	}

	switch metrics.MType {
	case models.Counter:
		if metrics.Delta == nil {
			return errors.ErrNilDelta
		}
		mm, ok := m.counter[metrics.ID]
		if ok && mm.Delta != nil {
			*metrics.Delta += *mm.Delta
		}
		m.counter[metrics.ID] = *metrics
	case models.Gauge:
		if metrics.Value == nil {
			return errors.ErrNilValue
		}
		m.gauge[metrics.ID] = *metrics
	default:
		return errors.ErrUnknownMetricType
	}
	return nil
}

func (m *MemStorage) Get(id, mType string) (*models.Metrics, error) {
	switch mType {
	case models.Counter:
		mm, ok := m.counter[id]
		if ok {
			return &mm, nil
		}
		return nil, errors.ErrNotFound
	case models.Gauge:
		mm, ok := m.gauge[id]
		if ok {
			return &mm, nil
		}
		return nil, errors.ErrNotFound
	default:
		return nil, errors.ErrUnknownMetricType
	}
}

func (m *MemStorage) GetAll() ([]*models.Metrics, error) {
	result := make([]*models.Metrics, 0, len(m.gauge)+len(m.counter))
	for _, v := range m.gauge {
		result = append(result, &v)
	}
	for _, v := range m.counter {
		result = append(result, &v)
	}
	return result, nil
}
