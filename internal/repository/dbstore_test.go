package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/AVZotov/metrics/internal/config/db"
	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("metrics"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("password"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Println("failed to start postgres container:", err)
		os.Exit(1)
	}
	defer func() {
		_ = pgContainer.Terminate(ctx)
	}()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Println("failed to get connection string:", err)
		os.Exit(1)
	}
	testDSN = connStr

	if err := RunMigrations(ctx, testDSN); err != nil {
		fmt.Println("failed to run migrations:", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func newTestDBStore(t *testing.T) *DBStore {
	t.Helper()

	cfg := &db.Config{
		ConnectTimeout: 5 * time.Second,
		QueryTimeout:   5 * time.Second,
	}
	store, err := NewDBStore(context.Background(), testDSN, cfg)
	if err != nil {
		t.Fatalf("failed to create DBStore: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = store.pool.Exec(ctx, "TRUNCATE TABLE metrics")
		store.Close()
	})
	return store
}

func TestDBStore_Ping(t *testing.T) {
	store := newTestDBStore(t)
	require.NoError(t, store.Ping(context.Background()))
}

func TestDBStore_Save_Get_Gauge(t *testing.T) {
	store := newTestDBStore(t)

	m := &models.Metrics{ID: "temp", MType: models.Gauge, Value: gaugePtr(3.14)}
	require.NoError(t, store.Save(m))

	got, err := store.Get("temp", models.Gauge)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "temp", got.ID)
	assert.Equal(t, models.Gauge, got.MType)
	assert.Equal(t, 3.14, *got.Value)
}

func TestDBStore_Save_Get_Counter(t *testing.T) {
	store := newTestDBStore(t)

	m := &models.Metrics{ID: "hits", MType: models.Counter, Delta: deltaPtr(42)}
	require.NoError(t, store.Save(m))

	got, err := store.Get("hits", models.Counter)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "hits", got.ID)
	assert.Equal(t, models.Counter, got.MType)
	assert.Equal(t, int64(42), *got.Delta)
}

func TestDBStore_Save_UpsertGauge(t *testing.T) {
	store := newTestDBStore(t)

	require.NoError(t, store.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(1.0)}))
	require.NoError(t, store.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(99.9)}))

	got, err := store.Get("cpu", models.Gauge)
	require.NoError(t, err)
	assert.Equal(t, 99.9, *got.Value)
}

func TestDBStore_Save_UpsertCounter(t *testing.T) {
	store := newTestDBStore(t)

	require.NoError(t, store.Save(&models.Metrics{ID: "reqs", MType: models.Counter, Delta: deltaPtr(5)}))
	require.NoError(t, store.Save(&models.Metrics{ID: "reqs", MType: models.Counter, Delta: deltaPtr(20)}))

	got, err := store.Get("reqs", models.Counter)
	require.NoError(t, err)
	// DBStore replaces delta on conflict; accumulation is MemStore's responsibility
	assert.Equal(t, int64(20), *got.Delta)
}

func TestDBStore_Get_NotFound(t *testing.T) {
	store := newTestDBStore(t)

	got, err := store.Get("missing", models.Gauge)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestDBStore_GetAll_Empty(t *testing.T) {
	store := newTestDBStore(t)

	all, err := store.GetAll()
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestDBStore_GetAll(t *testing.T) {
	store := newTestDBStore(t)

	require.NoError(t, store.Save(&models.Metrics{ID: "g1", MType: models.Gauge, Value: gaugePtr(1.0)}))
	require.NoError(t, store.Save(&models.Metrics{ID: "g2", MType: models.Gauge, Value: gaugePtr(2.0)}))
	require.NoError(t, store.Save(&models.Metrics{ID: "c1", MType: models.Counter, Delta: deltaPtr(10)}))

	all, err := store.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, 3)

	ids := make([]string, 0, len(all))
	for _, m := range all {
		ids = append(ids, m.ID)
	}
	assert.ElementsMatch(t, []string{"g1", "g2", "c1"}, ids)
}

func TestDBStore_SaveAll_Empty(t *testing.T) {
	store := newTestDBStore(t)
	require.NoError(t, store.SaveAll([]*models.Metrics{}))

	all, err := store.GetAll()
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestDBStore_SaveAll(t *testing.T) {
	store := newTestDBStore(t)

	batch := []*models.Metrics{
		{ID: "g1", MType: models.Gauge, Value: gaugePtr(1.1)},
		{ID: "c1", MType: models.Counter, Delta: deltaPtr(5)},
	}
	require.NoError(t, store.SaveAll(batch))

	all, err := store.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDBStore_SaveAll_Upsert(t *testing.T) {
	store := newTestDBStore(t)

	require.NoError(t, store.Save(&models.Metrics{ID: "g1", MType: models.Gauge, Value: gaugePtr(1.0)}))

	batch := []*models.Metrics{
		{ID: "g1", MType: models.Gauge, Value: gaugePtr(42.0)},
		{ID: "c1", MType: models.Counter, Delta: deltaPtr(7)},
	}
	require.NoError(t, store.SaveAll(batch))

	g1, err := store.Get("g1", models.Gauge)
	require.NoError(t, err)
	assert.Equal(t, 42.0, *g1.Value)

	c1, err := store.Get("c1", models.Counter)
	require.NoError(t, err)
	assert.Equal(t, int64(7), *c1.Delta)
}

func TestDBStore_Close(t *testing.T) {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		t.Skip("DATABASE_DSN not set; skipping DB integration test")
	}
	cfg := &db.Config{ConnectTimeout: 5 * time.Second}
	store, err := NewDBStore(context.Background(), dsn, cfg)
	require.NoError(t, err)
	assert.NoError(t, store.Close())
}
