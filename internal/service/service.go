package service

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

// Service is the business-logic layer handlers call into: validates and
// stores metrics, and reads them back.
type Service interface {
	UpdateMetric(metricType, name, value, ipAddress string) error
	UpdateMetrics(metrics []models.Metrics, ipAddress string) error
	GetMetric(id, mType string) (*models.Metrics, error)
	GetMetrics() ([]*models.Metrics, error)
}

// Pinger checks that the service's backing store is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// PersistService is a Service that can also be pinged — the shape exposed to handlers.
type PersistService interface {
	Service
	Pinger
}
