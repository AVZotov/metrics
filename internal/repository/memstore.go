package repository

import (
	"sync"

	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

var _ Repository = (*MemStore)(nil)

type MemStore struct {
	mu      sync.RWMutex
	gauge   map[string]models.Metrics
	counter map[string]models.Metrics
}

func NewMemStore() *MemStore {
	return &MemStore{
		gauge:   make(map[string]models.Metrics),
		counter: make(map[string]models.Metrics),
	}
}

func (m *MemStore) Save(metrics *models.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.save(metrics)
}

func (m *MemStore) Get(id, mType string) (*models.Metrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
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

func (m *MemStore) GetAll() ([]*models.Metrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*models.Metrics, 0, len(m.gauge)+len(m.counter))
	for _, v := range m.gauge {
		v := v
		result = append(result, &v)
	}
	for _, v := range m.counter {
		v := v
		result = append(result, &v)
	}
	return result, nil
}

func (m *MemStore) SaveAll(metrics []*models.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mm := range metrics {
		if err := m.save(mm); err != nil {
			return err
		}
	}
	return nil
}

// Save realize Delta incrementing as data reciever from the Agent
// PersistRepository using MemStore as single source of data without extra logic
// on DBStore or FileStore end
func (m *MemStore) save(metrics *models.Metrics) error {
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
