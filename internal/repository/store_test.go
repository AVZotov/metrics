package repository

import (
	"context"
	"errors"
	"testing"

	models "github.com/AVZotov/metrics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	saveErr   error
	getAllRet []*models.Metrics
	getAllErr error
}

func (m *mockRepo) Save(_ *models.Metrics) error             { return m.saveErr }
func (m *mockRepo) Get(_, _ string) (*models.Metrics, error) { return nil, nil }
func (m *mockRepo) GetAll() ([]*models.Metrics, error)       { return m.getAllRet, m.getAllErr }

type mockPersistRepo struct {
	mockRepo
	saveAllErr error
}

func (m *mockPersistRepo) SaveAll(_ []*models.Metrics) error { return m.saveAllErr }
func (m *mockPersistRepo) Close() error                      { return nil }
func (m *mockPersistRepo) Ping(_ context.Context) error      { return nil }

func TestNewStore(t *testing.T) {
	mem := NewMemStore()
	data := &mockPersistRepo{}
	s := NewStore(mem, data, true)
	require.NotNil(t, s)
	assert.True(t, s.syncMode)
	assert.Equal(t, mem, s.memStore)
	assert.Equal(t, data, s.persistStore)
}

func TestStore_Save_SyncMode_WritesToData(t *testing.T) {
	mem := NewMemStore()
	dir := t.TempDir()
	data, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	s := NewStore(mem, data, true)
	m := &models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(55.5)}
	require.NoError(t, s.Save(m))

	got, err := data.Get("cpu", models.Gauge)
	require.NoError(t, err)
	assert.Equal(t, 55.5, *got.Value)
}

func TestStore_Save_AsyncMode_DoesNotWriteToData(t *testing.T) {
	mem := NewMemStore()
	data := &mockPersistRepo{}
	s := NewStore(mem, data, false)

	require.NoError(t, s.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(1.0)}))

	all, err := data.GetAll()
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestStore_Save_MemError_Propagates(t *testing.T) {
	sentinel := errors.New("mem save error")
	s := NewStore(&mockRepo{saveErr: sentinel}, &mockPersistRepo{}, false)

	err := s.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(1.0)})
	assert.ErrorIs(t, err, sentinel)
}

func TestStore_Get_DelegatesToMem(t *testing.T) {
	mem := NewMemStore()
	v := 3.14
	mem.gauge["pi"] = models.Metrics{ID: "pi", MType: models.Gauge, Value: &v}

	s := NewStore(mem, &mockPersistRepo{}, false)
	got, err := s.Get("pi", models.Gauge)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 3.14, *got.Value)
}

func TestStore_GetAll_DelegatesToMem(t *testing.T) {
	mem := NewMemStore()
	v := 1.0
	d := int64(5)
	mem.gauge["g1"] = models.Metrics{ID: "g1", MType: models.Gauge, Value: &v}
	mem.counter["c1"] = models.Metrics{ID: "c1", MType: models.Counter, Delta: &d}

	s := NewStore(mem, &mockPersistRepo{}, false)
	all, err := s.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestStore_Dump(t *testing.T) {
	mem := NewMemStore()
	mem.gauge["cpu"] = models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(42.0)}
	mem.counter["hits"] = models.Metrics{ID: "hits", MType: models.Counter, Delta: deltaPtr(10)}

	dir := t.TempDir()
	data, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)

	s := NewStore(mem, data, false)
	require.NoError(t, s.Dump())

	all, err := data.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestStore_Dump_MemGetAllError(t *testing.T) {
	sentinel := errors.New("getall error")
	s := NewStore(&mockRepo{getAllErr: sentinel}, &mockPersistRepo{}, false)
	assert.ErrorIs(t, s.Dump(), sentinel)
}

func TestStore_Dump_DataSaveAllError(t *testing.T) {
	mem := NewMemStore()
	mem.gauge["g"] = models.Metrics{ID: "g", MType: models.Gauge, Value: gaugePtr(1.0)}

	sentinel := errors.New("data saveall error")
	s := NewStore(mem, &mockPersistRepo{saveAllErr: sentinel}, false)
	assert.ErrorIs(t, s.Dump(), sentinel)
}

func TestStore_Restore(t *testing.T) {
	dir := t.TempDir()
	data, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)
	require.NoError(t, data.Save(&models.Metrics{ID: "cpu", MType: models.Gauge, Value: gaugePtr(7.7)}))
	require.NoError(t, data.Save(&models.Metrics{ID: "reqs", MType: models.Counter, Delta: deltaPtr(3)}))

	mem := NewMemStore()
	s := NewStore(mem, data, false)
	require.NoError(t, s.Restore())

	all, err := mem.GetAll()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestStore_Restore_DataGetAllError(t *testing.T) {
	sentinel := errors.New("data getall error")
	s := NewStore(NewMemStore(), &mockPersistRepo{mockRepo: mockRepo{getAllErr: sentinel}}, false)
	assert.ErrorIs(t, s.Restore(), sentinel)
}

func TestStore_Restore_MemSaveError(t *testing.T) {
	dir := t.TempDir()
	data, err := NewFileStore("metrics.json", dir)
	require.NoError(t, err)
	require.NoError(t, data.Save(&models.Metrics{ID: "g", MType: models.Gauge, Value: gaugePtr(1.0)}))

	sentinel := errors.New("mem save error")
	s := NewStore(&mockRepo{saveErr: sentinel}, data, false)
	assert.ErrorIs(t, s.Restore(), sentinel)
}
