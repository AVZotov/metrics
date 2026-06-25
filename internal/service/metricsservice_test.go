package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

type mockRepo struct {
	saveFn   func(*models.Metrics) error
	getFn    func(string, string) (*models.Metrics, error)
	getAllFn func() ([]*models.Metrics, error)
}

func (r *mockRepo) Save(m *models.Metrics) error              { return r.saveFn(m) }
func (r *mockRepo) Get(id, t string) (*models.Metrics, error) { return r.getFn(id, t) }
func (r *mockRepo) GetAll() ([]*models.Metrics, error)        { return r.getAllFn() }
func (r *mockRepo) Ping(_ context.Context) error              { return nil }

func TestNewMetricsService(t *testing.T) {
	repo := &mockRepo{}
	svc := NewMetricsService(repo)
	require.NotNil(t, svc)
	assert.Equal(t, repo, svc.repository)
}

func TestMetricsService_UpdateMetric(t *testing.T) {
	tests := []struct {
		name       string
		metricType string
		metricName string
		value      string
		repoSaveFn func(*models.Metrics) error
		wantErr    error
		checkSaved func(t *testing.T, m *models.Metrics)
	}{
		{
			name:       "empty name returns ErrEmptyMetricName",
			metricType: models.Counter,
			metricName: "",
			value:      "1",
			wantErr:    errors.ErrEmptyMetricName,
		},
		{
			name:       "empty type returns ErrEmptyMetricType",
			metricType: "",
			metricName: "hits",
			value:      "1",
			wantErr:    errors.ErrEmptyMetricType,
		},
		{
			name:       "unknown type returns ErrUnknownMetricType",
			metricType: "unknown",
			metricName: "hits",
			value:      "1",
			wantErr:    errors.ErrUnknownMetricType,
		},
		{
			name:       "empty value returns ErrEmptyMetricValue",
			metricType: models.Counter,
			metricName: "hits",
			value:      "",
			wantErr:    errors.ErrEmptyMetricValue,
		},
		{
			name:       "counter with non-integer value returns ErrUnknownMetricValue",
			metricType: models.Counter,
			metricName: "hits",
			value:      "not-a-number",
			wantErr:    errors.ErrUnknownMetricValue,
		},
		{
			name:       "gauge with non-float value returns ErrUnknownMetricValue",
			metricType: models.Gauge,
			metricName: "temp",
			value:      "not-a-number",
			wantErr:    errors.ErrUnknownMetricValue,
		},
		{
			name:       "valid counter saves parsed delta",
			metricType: models.Counter,
			metricName: "hits",
			value:      "42",
			wantErr:    nil,
			checkSaved: func(t *testing.T, m *models.Metrics) {
				require.NotNil(t, m.Delta)
				assert.Equal(t, int64(42), *m.Delta)
				assert.Equal(t, "hits", m.ID)
				assert.Equal(t, models.Counter, m.MType)
			},
		},
		{
			name:       "valid gauge saves parsed value",
			metricType: models.Gauge,
			metricName: "temp",
			value:      "36.6",
			wantErr:    nil,
			checkSaved: func(t *testing.T, m *models.Metrics) {
				require.NotNil(t, m.Value)
				assert.Equal(t, 36.6, *m.Value)
				assert.Equal(t, "temp", m.ID)
				assert.Equal(t, models.Gauge, m.MType)
			},
		},
		{
			name:       "repository save error is propagated",
			metricType: models.Gauge,
			metricName: "temp",
			value:      "1.0",
			repoSaveFn: func(_ *models.Metrics) error { return errors.ErrNilValue },
			wantErr:    errors.ErrNilValue,
		},
	}
	
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				var saved *models.Metrics
				repo := &mockRepo{
					saveFn: func(m *models.Metrics) error {
						saved = m
						if tt.repoSaveFn != nil {
							return tt.repoSaveFn(m)
						}
						return nil
					},
				}
				err := NewMetricsService(repo).UpdateMetric(tt.metricType, tt.metricName, tt.value)
				if tt.wantErr != nil {
					assert.ErrorIs(t, err, tt.wantErr)
				} else {
					require.NoError(t, err)
					if tt.checkSaved != nil {
						tt.checkSaved(t, saved)
					}
				}
			},
		)
	}
}

func TestMetricsService_GetMetric(t *testing.T) {
	t.Run(
		"returns metric from repository", func(t *testing.T) {
			want := &models.Metrics{ID: "cpu", MType: models.Gauge, Value: new(3.14)}
			repo := &mockRepo{
				getFn: func(id, mType string) (*models.Metrics, error) {
					assert.Equal(t, "cpu", id)
					assert.Equal(t, models.Gauge, mType)
					return want, nil
				},
			}
			got, err := NewMetricsService(repo).GetMetric("cpu", models.Gauge)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		},
	)
	
	t.Run(
		"repository error", func(t *testing.T) {
			repo := &mockRepo{
				getFn: func(_, _ string) (*models.Metrics, error) {
					return nil, errors.ErrNotFound
				},
			}
			got, err := NewMetricsService(repo).GetMetric("missing", models.Counter)
			assert.ErrorIs(t, err, errors.ErrNotFound)
			assert.Nil(t, got)
		},
	)
}

func TestMetricsService_GetMetrics(t *testing.T) {
	t.Run(
		"returns all metrics from repo", func(t *testing.T) {
			want := []*models.Metrics{
				{ID: "g1", MType: models.Gauge, Value: new(1.0)},
				{ID: "c1", MType: models.Counter, Delta: new(int64(5))},
			}
			repo := &mockRepo{
				getAllFn: func() ([]*models.Metrics, error) { return want, nil },
			}
			got, err := NewMetricsService(repo).GetMetrics()
			require.NoError(t, err)
			assert.Equal(t, want, got)
		},
	)
	
	t.Run(
		"repository error", func(t *testing.T) {
			repo := &mockRepo{
				getAllFn: func() ([]*models.Metrics, error) { return nil, errors.ErrNotFound },
			}
			got, err := NewMetricsService(repo).GetMetrics()
			assert.ErrorIs(t, err, errors.ErrNotFound)
			assert.Nil(t, got)
		},
	)
}

func Test_parseInt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"valid positive", "42", 42, false},
		{"valid negative", "-7", -7, false},
		{"string", "abc", 0, true},
		{"float string", "3.14", 0, true},
	}
	
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := parseInt(tt.input)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.want, got)
				}
			},
		)
	}
}

func Test_parseFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{"valid float", "3.14", 3.14, false},
		{"integer string", "42", 42.0, false},
		{"negative float", "-1.5", -1.5, false},
		{"string", "abc", 0, true},
	}
	
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := parseFloat(tt.input)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.want, got)
				}
			},
		)
	}
}
