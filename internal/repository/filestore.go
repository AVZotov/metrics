package repository

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

var _ PersistRepository = (*FileStore)(nil)

// FileStore is a PersistRepository backed by a single JSON file on disk.
type FileStore struct {
	mu   sync.Mutex
	name string
	path string
}

// NewFileStore creates a FileStore writing to path/name, creating path if
// needed. Returns an error if the directory can't be created.
func NewFileStore(name, path string) (*FileStore, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}
	return &FileStore{
		name: name,
		path: path,
	}, nil
}

// Save updates m in place if a matching id/type already exists in the
// file, otherwise appends it, then rewrites the whole file. Returns an
// error if reading or writing the file fails.
func (d *FileStore) Save(m *models.Metrics) (err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	metrics, err := d.readAll()
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
	return d.writeAll(metrics)
}

// Get reads a single metric by id and type. Returns
// apperrors.ErrUnknownMetricType for an unrecognized mType, or
// apperrors.ErrNotFound if no matching metric exists.
func (d *FileStore) Get(id, mType string) (*models.Metrics, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if mType != models.Counter && mType != models.Gauge {
		return nil, apperrors.ErrUnknownMetricType
	}
	metrics, err := d.readAll()
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

// GetAll reads every metric from the file. Returns an empty slice (no
// error) if the file doesn't exist yet or is empty.
func (d *FileStore) GetAll() ([]*models.Metrics, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readAll()
}

func (d *FileStore) readAll() ([]*models.Metrics, error) {
	var metrics []*models.Metrics
	fullPath := filepath.Join(d.path, d.name)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return metrics, nil
		}
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	if err = json.NewDecoder(file).Decode(&metrics); err != nil {
		if errors.Is(err, io.EOF) {
			return metrics, nil
		}
		return nil, err
	}
	return metrics, nil
}

// SaveAll overwrites the file with the given metrics. Returns an error if
// marshaling or writing the file fails.
func (d *FileStore) SaveAll(metrics []*models.Metrics) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeAll(metrics)
}

// writeAll marshals metrics to JSON and atomically overwrites the store's
// file with them, via a temp file + rename so a crash mid-write can't
// leave a truncated/corrupt store
func (d *FileStore) writeAll(metrics []*models.Metrics) (err error) {
	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(d.path, "temp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()

	defer func() {
		_ = os.Remove(tmpName)
	}()
	defer func() {
		if closeErr := tmpFile.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	if _, err = tmpFile.Write(data); err != nil {
		return err
	}
	if err = tmpFile.Sync(); err != nil {
		return err
	}

	fullPath := filepath.Join(d.path, d.name)
	return os.Rename(tmpName, fullPath)
}

// Close is a no-op; FileStore doesn't keep the file open between writes.
func (d *FileStore) Close() error {
	return nil
}

// Ping always succeeds; a file-backed store has no connection to check.
func (d *FileStore) Ping(_ context.Context) error {
	return nil
}
