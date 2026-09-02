package service

import (
	"context"
	"strconv"

	"github.com/AVZotov/metrics/internal/audit"
	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/AVZotov/metrics/internal/repository"
)

var _ PersistService = (*MetricsService)(nil)

type metricsRepository interface {
	repository.Repository
	repository.Pinger
}

// MetricsService validates and persists metrics, and audits every write.
type MetricsService struct {
	repository metricsRepository
	notifier   *audit.Notifier
}

// NewMetricsService creates a MetricsService backed by r, auditing writes via n.
func NewMetricsService(r metricsRepository, n *audit.Notifier) *MetricsService {
	return &MetricsService{
		repository: r,
		notifier:   n,
	}
}

// UpdateMetric validates and saves a single metric given as strings, then
// audits the write. Returns errors.ErrEmptyMetricName/ErrEmptyMetricType/ErrEmptyMetricValue
// if a required field is blank, errors.ErrUnknownMetricType for an
// unrecognized type, errors.ErrUnknownMetricValue if value can't be
// parsed, or a repository error if the save fails.
func (m *MetricsService) UpdateMetric(metricType, name, value, ipAddress string) error {
	if name == "" {
		return errors.ErrEmptyMetricName
	}

	if metricType == "" {
		return errors.ErrEmptyMetricType
	}

	if metricType != models.Counter && metricType != models.Gauge {
		return errors.ErrUnknownMetricType
	}

	if value == "" {
		return errors.ErrEmptyMetricValue
	}

	metrics := &models.Metrics{
		ID:    name,
		MType: metricType,
	}

	switch metrics.MType {
	case models.Counter:
		v, err := parseInt(value)
		if err != nil {
			return errors.ErrUnknownMetricValue
		}
		metrics.Delta = &v
	case models.Gauge:
		v, err := parseFloat(value)
		if err != nil {
			return errors.ErrUnknownMetricValue
		}
		metrics.Value = &v
	}

	if err := m.repository.Save(metrics); err != nil {
		return err
	}

	event := audit.NewEvent([]string{metrics.ID}, ipAddress)
	m.notifier.Notify(event)

	return nil
}

// UpdateMetrics validates and saves a batch of metrics, then audits the
// write. Returns errors.ErrEmptyMetricName/ErrUnknownMetricType/ErrEmptyMetricValue
// if any metric is invalid, or a repository error if the save fails.
func (m *MetricsService) UpdateMetrics(metrics []models.Metrics, ipAddress string) error {
	toSave := make([]*models.Metrics, 0, len(metrics))
	auditNames := make([]string, 0, len(metrics))
	for i := range metrics {
		mm := &metrics[i]

		if mm.ID == "" {
			return errors.ErrEmptyMetricName
		}
		if mm.MType != models.Counter && mm.MType != models.Gauge {
			return errors.ErrUnknownMetricType
		}
		if mm.MType == models.Counter && mm.Delta == nil {
			return errors.ErrEmptyMetricValue
		}
		if mm.MType == models.Gauge && mm.Value == nil {
			return errors.ErrEmptyMetricValue
		}

		toSave = append(toSave, mm)
		auditNames = append(auditNames, mm.ID)
	}

	if err := m.repository.SaveAll(toSave); err != nil {
		return err
	}
	event := audit.NewEvent(auditNames, ipAddress)
	m.notifier.Notify(event)

	return nil
}

// GetMetric looks up a single metric by id and type. Returns
// errors.ErrNotFound if it doesn't exist.
func (m *MetricsService) GetMetric(id, mType string) (*models.Metrics, error) {
	return m.repository.Get(id, mType)
}

// GetMetrics returns every stored metric.
func (m *MetricsService) GetMetrics() ([]*models.Metrics, error) {
	return m.repository.GetAll()
}

// Ping checks that the backing repository is reachable.
func (m *MetricsService) Ping(ctx context.Context) error {
	return m.repository.Ping(ctx)
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
