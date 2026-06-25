package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
)

var _ Repository = (*Store)(nil)
var _ Closer = (*Store)(nil)
var _ Pinger = (*Store)(nil)

type Store struct {
	memStore     Repository
	persistStore PersistRepository
	syncMode     bool
}

func NewStore(memRepo Repository, dataRepo PersistRepository, syncMode bool) *Store {
	return &Store{
		memStore:     memRepo,
		persistStore: dataRepo,
		syncMode:     syncMode,
	}
}

func (s *Store) Save(m *models.Metrics) error {
	if err := s.memStore.Save(m); err != nil {
		return err
	}
	if s.syncMode {
		return s.Dump()
	}
	return nil
}

func (s *Store) Get(id, mType string) (*models.Metrics, error) {
	return s.memStore.Get(id, mType)
}

func (s *Store) GetAll() ([]*models.Metrics, error) {
	return s.memStore.GetAll()
}

func (s *Store) Dump() error {
	metrics, err := s.memStore.GetAll()
	if err != nil {
		return err
	}
	return s.persistStore.SaveAll(metrics)
}

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

func (s *Store) Close() error {
	return s.persistStore.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.persistStore.Ping(ctx)
}
