package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

var _ Repository = (*Store)(nil)
var _ Closer = (*Store)(nil)
var _ Pinger = (*Store)(nil)

// Store combines an in-memory Repository with a durable PersistRepository,
// dumping to the persistent store on every write when syncMode is on.
type Store struct {
	memStore     Repository
	persistStore PersistRepository
	syncMode     bool
}

// NewStore creates a Store backed by memRepo for reads/writes and dataRepo
// for persistence. If syncMode is true, every Save/SaveAll also dumps to dataRepo.
func NewStore(memRepo Repository, dataRepo PersistRepository, syncMode bool) *Store {
	return &Store{
		memStore:     memRepo,
		persistStore: dataRepo,
		syncMode:     syncMode,
	}
}

// Save writes m to the in-memory store, then dumps to the persistent store
// if syncMode is on. Returns an error if either write fails.
func (s *Store) Save(m *models.Metrics) error {
	if err := s.memStore.Save(m); err != nil {
		return err
	}
	if s.syncMode {
		return s.Dump()
	}
	return nil
}

// SaveAll writes metrics to the in-memory store, then dumps to the
// persistent store if syncMode is on. Returns an error if either write fails.
func (s *Store) SaveAll(metrics []*models.Metrics) error {
	if err := s.memStore.SaveAll(metrics); err != nil {
		return err
	}
	if s.syncMode {
		return s.Dump()
	}
	return nil
}

// Get reads a single metric from the in-memory store. Returns
// apperrors.ErrNotFound if it doesn't exist.
func (s *Store) Get(id, mType string) (*models.Metrics, error) {
	return s.memStore.Get(id, mType)
}

// GetAll returns every metric from the in-memory store.
func (s *Store) GetAll() ([]*models.Metrics, error) {
	return s.memStore.GetAll()
}

// Dump copies every metric from the in-memory store into the persistent
// store. Returns an error if reading or the persistent write fails.
func (s *Store) Dump() error {
	metrics, err := s.memStore.GetAll()
	if err != nil {
		return err
	}
	return s.persistStore.SaveAll(metrics)
}

// Restore loads every metric from the persistent store into the in-memory
// store. Returns an error if reading the persistent store or writing to memory fails.
func (s *Store) Restore() error {
	metrics, err := s.persistStore.GetAll()
	if err != nil {
		return err
	}
	for _, m := range metrics {
		if err := s.memStore.Save(m); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the persistent store.
func (s *Store) Close() error {
	return s.persistStore.Close()
}

// Ping checks that the persistent store is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.persistStore.Ping(ctx)
}
