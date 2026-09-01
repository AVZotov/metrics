package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

var _ PersistRepository = (*NoopStore)(nil)

// NoopStore is a PersistRepository that discards everything — used when no
// persistence backend (file or DB) is configured.
type NoopStore struct{}

// NewNoopStore returns a PersistRepository that does nothing.
func NewNoopStore() PersistRepository { return NoopStore{} }

// Save does nothing and always succeeds.
func (NoopStore) Save(_ *models.Metrics) error { return nil }

// Get always returns a nil metric and no error.
func (NoopStore) Get(_, _ string) (*models.Metrics, error) { return nil, nil }

// GetAll always returns a nil slice and no error.
func (NoopStore) GetAll() ([]*models.Metrics, error) { return nil, nil }

// SaveAll does nothing and always succeeds.
func (NoopStore) SaveAll(_ []*models.Metrics) error { return nil }

// Close does nothing and always succeeds.
func (NoopStore) Close() error { return nil }

// Ping does nothing and always succeeds.
func (NoopStore) Ping(_ context.Context) error { return nil }
