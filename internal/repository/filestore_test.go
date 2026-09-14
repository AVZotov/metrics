package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gaugePtr(v float64) *float64 { return &v }
func deltaPtr(v int64) *int64     { return &v }

func TestNewDataStore(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)
	require.NotNil(t, ds)
	assert.Equal(t, "metrics.json", ds.name)
	assert.Equal(t, dir, ds.path)
}

func TestNewDataStore_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")
	ds, err := NewFileStore("metrics.json", nested)
	require.NoError(t, err)
	require.NotNil(t, ds)
	_, statErr := os.Stat(nested)
	require.NoError(t, statErr)
}

func TestDataStore_GetAll_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	got, err := ds.GetAll()
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestDataStore_Save_NewMetric(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	m := &models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(42.5)}
	require.NoError(t, ds.Save(m))

	all, err := ds.GetAll()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "cpu", all[0].ID)
	assert.Equal(t, models.Gauge, all[0].MType)
	assert.Equal(t, 42.5, *all[0].Value)
}

func TestDataStore_Save_UpdatesExistingGauge(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	require.NoError(t, ds.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(1.0)}))
	require.NoError(t, ds.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(99.9)}))

	all, err := ds.GetAll()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, 99.9, *all[0].Value)
}

func TestDataStore_Save_UpdatesExistingCounter(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	require.NoError(t, ds.Save(&models.Metrics{ID: "hits", MType: models.Counter, Delta: deltaPtr(10)}))
	require.NoError(t, ds.Save(&models.Metrics{ID: "hits", MType: models.Counter, Delta: deltaPtr(20)}))

	all, err := ds.GetAll()
	require.NoError(t, err)
	require.Len(t, all, 1)
	// DataStore replaces delta (accumulation is MemStorage's responsibility)
	assert.Equal(t, int64(20), *all[0].Delta)
}

func TestDataStore_Save_AppendsNewMetric(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	require.NoError(t, ds.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(1.0)}))
	require.NoError(t, ds.Save(&models.Metrics{ID: "hits", MType: models.Counter, Delta: deltaPtr(5)}))

	all, err := ds.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDataStore_Save_UpdatesHash(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	require.NoError(t, ds.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(1.0), Hash: "old"}))
	require.NoError(t, ds.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(2.0), Hash: "new"}))

	all, err := ds.GetAll()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "new", all[0].Hash)
}

func TestDataStore_Get(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	require.NoError(t, ds.Save(&models.Metrics{ID: "pi", MType: models.Gauge, Value: gaugePtr(3.14)}))
	require.NoError(t, ds.Save(&models.Metrics{ID: "reqs", MType: models.Counter, Delta: deltaPtr(7)}))

	tests := []struct {
		name    string
		id      string
		mType   string
		wantErr error
	}{
		{
			name:  "get existing gauge",
			id:    "pi",
			mType: models.Gauge,
		},
		{
			name:  "get existing counter",
			id:    "reqs",
			mType: models.Counter,
		},
		{
			name:    "missing metric returns ErrNotFound",
			id:      "missing",
			mType:   models.Gauge,
			wantErr: apperrors.ErrNotFound,
		},
		{
			name:    "unknown type returns ErrUnknownMetricType",
			id:      "pi",
			mType:   "unknown",
			wantErr: apperrors.ErrUnknownMetricType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Get(tt.id, tt.mType)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.id, got.ID)
			}
		})
	}
}

func TestDataStore_Get_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	_, err = ds.Get("any", models.Gauge)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// TestFileStore_ConcurrentSave_NoCorruption exercises the read-modify-write
// race Save's mutex closes: without it, concurrent Save calls can each read
// the file before the others' writes land and clobber each other's entries,
// or GetAll can read a file mid-truncate. Run with -race.
func TestFileStore_ConcurrentSave_NoCorruption(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("metric-%d", i)
			assert.NoError(t, ds.Save(&models.Metrics{ID: id, MType: models.Gauge, Value: gaugePtr(float64(i))}))
		}()
	}
	wg.Wait()

	all, err := ds.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, goroutines, "every concurrent Save should have survived without clobbering the others")
}

// TestFileStore_ConcurrentSaveAndGetAll checks that a GetAll running
// concurrently with Saves never observes a partially written (corrupted)
// file, since readAll and Save now share the same mutex.
func TestFileStore_ConcurrentSaveAndGetAll(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("metric-%d", i)
			assert.NoError(t, ds.Save(&models.Metrics{ID: id, MType: models.Gauge, Value: gaugePtr(float64(i))}))
		}()
		go func() {
			defer wg.Done()
			_, err := ds.GetAll()
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	all, err := ds.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, goroutines)
}

// TestFileStore_ConcurrentSaveAll checks that concurrent SaveAll calls, which
// each fully overwrite the file, never leave behind a partially written file
// that GetAll then fails to decode.
func TestFileStore_ConcurrentSaveAll(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			metrics := []*models.Metrics{
				{ID: fmt.Sprintf("m%d", i), MType: models.Gauge, Value: gaugePtr(float64(i))},
			}
			assert.NoError(t, ds.SaveAll(metrics))
		}()
	}
	wg.Wait()

	all, err := ds.GetAll()
	require.NoError(t, err, "file should be valid JSON, not corrupted by interleaved writes")
	require.Len(t, all, 1, "SaveAll always overwrites, so exactly one goroutine's write should be the final state")
}
