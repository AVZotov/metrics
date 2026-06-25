package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

var _ PersistRepository = (*DBStore)(nil)

type DBStore struct {
}

func NewDBStore() *DBStore {
	return &DBStore{}
}

func (d *DBStore) Save(metrics *models.Metrics) error {
	panic("implement me")
}

func (d *DBStore) Get(id, mType string) (*models.Metrics, error) {
	panic("implement me")
}

func (d *DBStore) GetAll() ([]*models.Metrics, error) {
	panic("implement me")
}

func (d *DBStore) SaveAll(metrics []*models.Metrics) error {
	panic("implement me")
}

func (d *DBStore) Close() error {
	panic("implement me")
}

func (d *DBStore) Ping(_ context.Context) error {
	panic("implement me")
}
