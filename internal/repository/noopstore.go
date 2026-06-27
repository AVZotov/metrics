package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

var _ PersistRepository = (*noopStore)(nil)

type noopStore struct{}

func NewNoopStore() PersistRepository { return noopStore{} }

func (noopStore) Save(_ *models.Metrics) error             { return nil }
func (noopStore) Get(_, _ string) (*models.Metrics, error) { return nil, nil }
func (noopStore) GetAll() ([]*models.Metrics, error)       { return nil, nil }
func (noopStore) SaveAll(_ []*models.Metrics) error        { return nil }
func (noopStore) Close() error                             { return nil }
func (noopStore) Ping(_ context.Context) error             { return nil }
