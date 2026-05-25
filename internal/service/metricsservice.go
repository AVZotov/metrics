package service

import (
	"strconv"

	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/AVZotov/metrics/internal/repository"
)

var _ Service = (*MetricsService)(nil)

type MetricsService struct {
	repository repository.Repository
}

func NewMetricsService(r repository.Repository) *MetricsService {
	return &MetricsService{
		repository: r,
	}
}

func (m *MetricsService) UpdateMetric(metricType, name, value string) error {
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

	return nil
}

func (m *MetricsService) GetMetric(id, mType string) (*models.Metrics, error) {
	return m.repository.Get(id, mType)
}

func (m *MetricsService) GetMetrics() ([]*models.Metrics, error) {
	return m.repository.GetAll()
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
