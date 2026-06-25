package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

type Repository interface {
	Save(metrics *models.Metrics) error
	Get(id, mType string) (*models.Metrics, error)
	GetAll() ([]*models.Metrics, error)
}

type BulkSaver interface {
	SaveAll(metrics []*models.Metrics) error
}

type Closer interface {
	Close() error
}

type Pinger interface {
	Ping(ctx context.Context) error
}

type PersistRepository interface {
	Repository
	BulkSaver
	Closer
	Pinger
}
