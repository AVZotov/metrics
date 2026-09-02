package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

// Repository stores and retrieves metrics. Get and GetAll return
// apperrors.ErrNotFound / apperrors.ErrUnknownMetricType (implementation-dependent) when a metric isn't found.
type Repository interface {
	Save(metrics *models.Metrics) error
	SaveAll(metrics []*models.Metrics) error
	Get(id, mType string) (*models.Metrics, error)
	GetAll() ([]*models.Metrics, error)
}

// Closer releases resources held by a repository.
type Closer interface {
	Close() error
}

// Pinger checks that a repository's backing store is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// PersistRepository is a Repository that can also be closed and pinged —
// the shape expected of a durable (non-memory) store.
type PersistRepository interface {
	Repository
	Closer
	Pinger
}
