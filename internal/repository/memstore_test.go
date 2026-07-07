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
	s := NewMemStore()
	require.NotNil(t, s)
	assert.NotNil(t, s.gauge)
	assert.NotNil(t, s.counter)
}

func TestMemStorage_Save(t *testing.T) {
	tests := []struct {
		name    string
		initial func() *MemStore
		metric  *models.Metrics
		wantErr error
		check   func(t *testing.T, s *MemStore)
	}{
		{
			name:    "nil metric returns ErrNilMetric",
			initial: NewMemStore,
			metric:  nil,
			wantErr: errors.ErrNilMetric,
		},
		{
			name:    "unknown type returns ErrUnknownMetricType",
			initial: NewMemStore,
			metric:  &models.Metrics{ID: "x", MType: "unknown"},
			wantErr: errors.ErrUnknownMetricType,
		},
		{
			name:    "counter with nil delta returns ErrNilDelta",
			initial: NewMemStore,
			metric:  &models.Metrics{ID: "c", MType: models.Counter, Delta: nil},
			wantErr: errors.ErrNilDelta,
		},
		{
			name:    "gauge with nil value returns ErrNilValue",
			initial: NewMemStore,
			metric:  &models.Metrics{ID: "g", MType: models.Gauge, Value: nil},
			wantErr: errors.ErrNilValue,
		},
		{
			name:    "save gauge stores value",
			initial: NewMemStore,
			metric:  &models.Metrics{ID: "temp", MType: models.Gauge, Value: new(36.6)},
			wantErr: nil,
			check: func(t *testing.T, s *MemStore) {
				m, ok := s.gauge["temp"]
				require.True(t, ok)
				assert.Equal(t, 36.6, *m.Value)
			},
		},
		{
			name:    "save gauge overwrites previous value",
			initial: NewMemStore,
			metric:  &models.Metrics{ID: "temp", MType: models.Gauge, Value: new(100.0)},
			wantErr: nil,
			check: func(t *testing.T, s *MemStore) {
				m, ok := s.gauge["temp"]
				require.True(t, ok)
				assert.Equal(t, 100.0, *m.Value)
			},
		},
		{
			name:    "save counter stores delta",
			initial: NewMemStore,
			metric:  &models.Metrics{ID: "hits", MType: models.Counter, Delta: new(int64(5))},
			wantErr: nil,
			check: func(t *testing.T, s *MemStore) {
				m, ok := s.counter["hits"]
				require.True(t, ok)
				assert.Equal(t, int64(5), *m.Delta)
			},
		},
		{
			name: "save counter accumulates delta",
			initial: func() *MemStore {
				s := NewMemStore()
				s.counter["count"] = models.Metrics{ID: "count", MType: models.Counter, Delta: new(int64(3))}
				return s
			},
			metric:  &models.Metrics{ID: "count", MType: models.Counter, Delta: new(int64(7))},
			wantErr: nil,
			check: func(t *testing.T, s *MemStore) {
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
	s := NewMemStore()
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
			s := NewMemStore()
			result, err := s.GetAll()
			require.NoError(t, err)
			assert.Empty(t, result)
		},
	)
	
	t.Run(
		"returns all gauges and counters", func(t *testing.T) {
			s := NewMemStore()
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

func TestMemStorage_SaveAll(t *testing.T) {
	t.Run(
		"saves multiple metrics", func(t *testing.T) {
			s := NewMemStore()
			d := int64(7)
			v := 1.5
			metrics := []*models.Metrics{
				{ID: "hits", MType: models.Counter, Delta: &d},
				{ID: "temp", MType: models.Gauge, Value: &v},
			}
			require.NoError(t, s.SaveAll(metrics))

			got, err := s.GetAll()
			require.NoError(t, err)
			assert.Len(t, got, 2)
		},
	)

	t.Run(
		"counter delta accumulates across batch items", func(t *testing.T) {
			s := NewMemStore()
			d1, d2 := int64(3), int64(7)
			metrics := []*models.Metrics{
				{ID: "hits", MType: models.Counter, Delta: &d1},
				{ID: "hits", MType: models.Counter, Delta: &d2},
			}
			require.NoError(t, s.SaveAll(metrics))

			got, err := s.Get("hits", models.Counter)
			require.NoError(t, err)
			assert.Equal(t, int64(10), *got.Delta)
		},
	)

	t.Run(
		"stops on first error", func(t *testing.T) {
			s := NewMemStore()
			metrics := []*models.Metrics{
				nil,
				{ID: "temp", MType: models.Gauge, Value: new(1.0)},
			}
			err := s.SaveAll(metrics)
			assert.ErrorIs(t, err, errors.ErrNilMetric)

			all, _ := s.GetAll()
			assert.Empty(t, all)
		},
	)
}

// Need to run with -race detector for full coverage
// Yandex git:(iter4) go test -race ./...
// fatal error: concurrent map writes without mutex
func TestMemStorage_ConcurrentSave(t *testing.T) {
	s := NewMemStore()
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
