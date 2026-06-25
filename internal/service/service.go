package service

import (
	"context"
	
	models "github.com/AVZotov/metrics/internal/model"
)

type Service interface {
	UpdateMetric(metricType, name, value string) error
	GetMetric(id, mType string) (*models.Metrics, error)
	GetMetrics() ([]*models.Metrics, error)
}

type Pinger interface {
	Ping(ctx context.Context) error
}

type PersistService interface {
	Service
	Pinger
}
