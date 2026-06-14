package repository

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	
	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

var _ Repository = (*DataStore)(nil)

type DataStore struct {
	name string
	path string
}

func NewDataStore(name, path string) (*DataStore, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}
	return &DataStore{
		name: name,
		path: path,
	}, nil
}

func (d *DataStore) Save(m *models.Metrics) error {
	metrics, err := d.GetAll()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
			
		}
	}
	var found bool
	for i, mm := range metrics {
		if mm.ID == m.ID && mm.MType == m.MType {
			switch mm.MType {
			case models.Counter:
				metrics[i].Delta = m.Delta
			case models.Gauge:
				metrics[i].Value = m.Value
			}
			metrics[i].Hash = m.Hash
			found = true
		}
	}
	if !found {
		metrics = append(metrics, m)
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}
	fullPath := filepath.Join(d.path, d.name)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func (d *DataStore) Get(id, mType string) (*models.Metrics, error) {
	if mType != models.Counter && mType != models.Gauge {
		return nil, apperrors.ErrUnknownMetricType
	}
	metrics, err := d.GetAll()
	if err != nil {
		return nil, err
	}
	for _, mm := range metrics {
		if mm.ID == id && mm.MType == mType {
			return mm, nil
		}
	}
	return nil, apperrors.ErrNotFound
}

func (d *DataStore) GetAll() ([]*models.Metrics, error) {
	fullPath := filepath.Join(d.path, d.name)
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var metrics []*models.Metrics
	if err = json.NewDecoder(file).Decode(&metrics); err != nil {
		if errors.Is(err, io.EOF) {
			return metrics, nil
		}
		return nil, err
	}
	return metrics, nil
}
