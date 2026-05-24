package repository

import (
	"sync"
	"testing"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

func TestNewMemStorage(t *testing.T) {
	s := NewMemStorage()
	require.NotNil(t, s)
	assert.NotNil(t, s.gauge)
	assert.NotNil(t, s.counter)
}

func TestMemStorage_Save(t *testing.T) {
	tests := []struct {
		name    string
		initial func() *MemStorage
		metric  *models.Metrics
		wantErr error
		check   func(t *testing.T, s *MemStorage)
	}{
		{
			name:    "nil metric returns ErrNilMetric",
			initial: NewMemStorage,
			metric:  nil,
			wantErr: errors.ErrNilMetric,
		},
		{
			name:    "unknown type returns ErrUnknownMetricType",
			initial: NewMemStorage,
			metric:  &models.Metrics{ID: "x", MType: "unknown"},
			wantErr: errors.ErrUnknownMetricType,
		},
		{
			name:    "counter with nil delta returns ErrNilDelta",
			initial: NewMemStorage,
			metric:  &models.Metrics{ID: "c", MType: models.Counter, Delta: nil},
			wantErr: errors.ErrNilDelta,
		},
		{
			name:    "gauge with nil value returns ErrNilValue",
			initial: NewMemStorage,
			metric:  &models.Metrics{ID: "g", MType: models.Gauge, Value: nil},
			wantErr: errors.ErrNilValue,
		},
		{
			name:    "save gauge stores value",
			initial: NewMemStorage,
			metric:  &models.Metrics{ID: "temp", MType: models.Gauge, Value: new(36.6)},
			wantErr: nil,
			check: func(t *testing.T, s *MemStorage) {
				m, ok := s.gauge["temp"]
				require.True(t, ok)
				assert.Equal(t, 36.6, *m.Value)
			},
		},
		{
			name:    "save gauge overwrites previous value",
			initial: NewMemStorage,
			metric:  &models.Metrics{ID: "temp", MType: models.Gauge, Value: new(100.0)},
			wantErr: nil,
			check: func(t *testing.T, s *MemStorage) {
				m, ok := s.gauge["temp"]
				require.True(t, ok)
				assert.Equal(t, 100.0, *m.Value)
			},
		},
		{
			name:    "save counter stores delta",
			initial: NewMemStorage,
			metric:  &models.Metrics{ID: "hits", MType: models.Counter, Delta: new(int64(5))},
			wantErr: nil,
			check: func(t *testing.T, s *MemStorage) {
				m, ok := s.counter["hits"]
				require.True(t, ok)
				assert.Equal(t, int64(5), *m.Delta)
			},
		},
		{
			name: "save counter accumulates delta",
			initial: func() *MemStorage {
				s := NewMemStorage()
				s.counter["count"] = models.Metrics{ID: "count", MType: models.Counter, Delta: new(int64(3))}
				return s
			},
			metric:  &models.Metrics{ID: "count", MType: models.Counter, Delta: new(int64(7))},
			wantErr: nil,
			check: func(t *testing.T, s *MemStorage) {
				m, ok := s.counter["count"]
				require.True(t, ok)
				assert.Equal(t, int64(10), *m.Delta)
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				s := tt.initial()
				err := s.Save(tt.metric)
				if tt.wantErr != nil {
					assert.ErrorIs(t, err, tt.wantErr)
				} else {
					require.NoError(t, err)
					if tt.check != nil {
						tt.check(t, s)
					}
				}
			},
		)
	}
}

func TestMemStorage_Get(t *testing.T) {
	s := NewMemStorage()
	s.counter["reqs"] = models.Metrics{ID: "reqs", MType: models.Counter, Delta: new(int64(42))}
	s.gauge["cpu"] = models.Metrics{ID: "cpu", MType: models.Gauge, Value: new(3.14)}
	
	tests := []struct {
		name    string
		id      string
		mType   string
		wantID  string
		wantErr error
	}{
		{
			name:    "get existing counter",
			id:      "reqs",
			mType:   models.Counter,
			wantID:  "reqs",
			wantErr: nil,
		},
		{
			name:    "get missing counter returns ErrNotFound",
			id:      "missing",
			mType:   models.Counter,
			wantErr: errors.ErrNotFound,
		},
		{
			name:    "get existing gauge",
			id:      "cpu",
			mType:   models.Gauge,
			wantID:  "cpu",
			wantErr: nil,
		},
		{
			name:    "get missing gauge returns ErrNotFound",
			id:      "missing",
			mType:   models.Gauge,
			wantErr: errors.ErrNotFound,
		},
		{
			name:    "unknown type returns ErrUnknownMetricType",
			id:      "any",
			mType:   "unknown",
			wantErr: errors.ErrUnknownMetricType,
		},
	}
	
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := s.Get(tt.id, tt.mType)
				if tt.wantErr != nil {
					assert.ErrorIs(t, err, tt.wantErr)
					assert.Nil(t, got)
				} else {
					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, tt.wantID, got.ID)
				}
			},
		)
	}
}

func TestMemStorage_GetAll(t *testing.T) {
	t.Run(
		"empty storage returns empty slice", func(t *testing.T) {
			s := NewMemStorage()
			result, err := s.GetAll()
			require.NoError(t, err)
			assert.Empty(t, result)
		},
	)
	
	t.Run(
		"returns all gauges and counters", func(t *testing.T) {
			s := NewMemStorage()
			s.gauge["g1"] = models.Metrics{ID: "g1", MType: models.Gauge, Value: new(1.0)}
			s.gauge["g2"] = models.Metrics{ID: "g2", MType: models.Gauge, Value: new(2.0)}
			s.counter["c1"] = models.Metrics{ID: "c1", MType: models.Counter, Delta: new(int64(10))}
			
			result, err := s.GetAll()
			require.NoError(t, err)
			assert.Len(t, result, 3)
			
			ids := make([]string, 0, len(result))
			for _, m := range result {
				ids = append(ids, m.ID)
			}
			assert.ElementsMatch(t, []string{"g1", "g2", "c1"}, ids)
		},
	)
}

// Need to run with -race detector for full coverage
// Yandex git:(iter4) go test -race ./...
// fatal error: concurrent map writes without mutex
func TestMemStorage_ConcurrentSave(t *testing.T) {
	s := NewMemStorage()
	const requests int64 = 1000
	
	tests := []struct {
		name string
		want int64
	}{
		{
			name: "concurrent save must return N requests",
			want: requests,
		},
	}
	
	for _, tt := range tests {
		var wg sync.WaitGroup
		t.Run(
			tt.name, func(t *testing.T) {
				for i := 0; i < int(requests); i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := s.Save(
							&models.Metrics{
								ID:    "counter",
								MType: models.Counter,
								Delta: new(int64(1)),
							},
						)
						if err != nil {
							assert.NoError(t, err)
						}
					}()
				}
				wg.Wait()
			},
		)
	}
	assert.Equal(t, requests, *s.counter["counter"].Delta)
}
