package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

var _ PersistRepository = (*NoopStore)(nil)

type NoopStore struct{}

func NewNoopStore() PersistRepository { return NoopStore{} }

func (NoopStore) Save(_ *models.Metrics) error             { return nil }
func (NoopStore) Get(_, _ string) (*models.Metrics, error) { return nil, nil }
func (NoopStore) GetAll() ([]*models.Metrics, error)       { return nil, nil }
func (NoopStore) SaveAll(_ []*models.Metrics) error        { return nil }
func (NoopStore) Close() error                             { return nil }
func (NoopStore) Ping(_ context.Context) error             { return nil }
