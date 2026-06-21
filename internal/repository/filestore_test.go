package repository

import (
	"os"
	"path/filepath"
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
	ds, err := NewDataStore("metrics.json", dir)
	require.NoError(t, err)
	require.NotNil(t, ds)
	assert.Equal(t, "metrics.json", ds.name)
	assert.Equal(t, dir, ds.path)
}

func TestNewDataStore_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")
	ds, err := NewDataStore("metrics.json", nested)
	require.NoError(t, err)
	require.NotNil(t, ds)
	_, statErr := os.Stat(nested)
	require.NoError(t, statErr)
}

func TestDataStore_GetAll_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewDataStore("metrics.json", dir)
	require.NoError(t, err)

	_, err = ds.GetAll()
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestDataStore_Save_NewMetric(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewDataStore("metrics.json", dir)
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
	ds, err := NewDataStore("metrics.json", dir)
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
	ds, err := NewDataStore("metrics.json", dir)
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
	ds, err := NewDataStore("metrics.json", dir)
	require.NoError(t, err)

	require.NoError(t, ds.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(1.0)}))
	require.NoError(t, ds.Save(&models.Metrics{ID: "hits", MType: models.Counter, Delta: deltaPtr(5)}))

	all, err := ds.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDataStore_Save_UpdatesHash(t *testing.T) {
	dir := t.TempDir()
	ds, err := NewDataStore("metrics.json", dir)
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
	ds, err := NewDataStore("metrics.json", dir)
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
	ds, err := NewDataStore("metrics.json", dir)
	require.NoError(t, err)

	_, err = ds.Get("any", models.Gauge)
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}
