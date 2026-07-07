package service

import (
	"context"
	"strconv"
	
	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/AVZotov/metrics/internal/repository"
)

var _ PersistService = (*MetricsService)(nil)

type metricsRepository interface {
	repository.Repository
	repository.Pinger
}

type MetricsService struct {
	repository metricsRepository
}

func NewMetricsService(r metricsRepository) *MetricsService {
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

func (m *MetricsService) UpdateMetrics(metrics []models.Metrics) error {
	toSave := make([]*models.Metrics, 0, len(metrics))
	for i := range metrics {
		mm := metrics[i]
		
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
		
		toSave = append(toSave, &mm)
	}
	
	return m.repository.SaveAll(toSave)
}

func (m *MetricsService) GetMetric(id, mType string) (*models.Metrics, error) {
	return m.repository.Get(id, mType)
}

func (m *MetricsService) GetMetrics() ([]*models.Metrics, error) {
	return m.repository.GetAll()
}

func (m *MetricsService) Ping(ctx context.Context) error {
	return m.repository.Ping(ctx)
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
