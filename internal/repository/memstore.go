package repository

import (
	"sync"

	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

var _ Repository = (*MemStore)(nil)

// MemStore is an in-memory Repository. Counter values accumulate on Save;
// gauge values are overwritten.
type MemStore struct {
	mu      sync.RWMutex
	gauge   map[string]models.Metrics
	counter map[string]models.Metrics
}

// NewMemStore creates an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		gauge:   make(map[string]models.Metrics),
		counter: make(map[string]models.Metrics),
	}
}

// Save stores metrics, accumulating the delta if it's a counter. Returns
// errors.ErrNilMetric, errors.ErrNilDelta/ErrNilValue, or errors.ErrUnknownMetricType as appropriate.
func (m *MemStore) Save(metrics *models.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.save(metrics)
}

// Get returns a single metric by id and type. Returns errors.ErrNotFound if
// it doesn't exist, or errors.ErrUnknownMetricType for an unrecognized mType.
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

// GetAll returns every stored gauge and counter metric.
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

// SaveAll saves each metric in order, stopping at the first error.
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
		total := *metrics.Delta
		if mm, ok := m.counter[metrics.ID]; ok && mm.Delta != nil {
			total += *mm.Delta
		}
		stored := *metrics
		stored.Delta = &total
		m.counter[metrics.ID] = stored
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
