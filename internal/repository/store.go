package repository

import models "github.com/AVZotov/metrics/internal/model"

var _ Repository = (*Store)(nil)

type Store struct {
	mem      Repository
	data     Repository
	syncMode bool
}

func NewStore(memRepo, dataRepo Repository, syncMode bool) *Store {
	return &Store{
		mem:      memRepo,
		data:     dataRepo,
		syncMode: syncMode,
	}
}

func (s *Store) Save(m *models.Metrics) error {
	if err := s.mem.Save(m); err != nil {
		return err
	}
	if s.syncMode {
		return s.Dump()
	}
	return nil
}

func (s *Store) Get(id, mType string) (*models.Metrics, error) {
	return s.mem.Get(id, mType)
}

func (s *Store) GetAll() ([]*models.Metrics, error) {
	return s.mem.GetAll()
}

func (s *Store) Dump() error {
	metrics, err := s.mem.GetAll()
	if err != nil {
		return err
	}
	for _, m := range metrics {
		err = s.data.Save(m)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Restore() error {
	metrics, err := s.data.GetAll()
	if err != nil {
		return err
	}
	for _, m := range metrics {
		err = s.mem.Save(m)
		if err != nil {
			return err
		}
	}
	return nil
}
