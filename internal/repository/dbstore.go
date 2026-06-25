package repository

import (
	"context"

	models "github.com/AVZotov/metrics/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ PersistRepository = (*DBStore)(nil)

type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(ctx context.Context, dsn string) (*DBStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &DBStore{
		pool: pool,
	}, nil
}

func (d *DBStore) Save(metrics *models.Metrics) error {
	panic("implement me")
}

func (d *DBStore) Get(id, mType string) (*models.Metrics, error) {
	panic("implement me")
}

func (d *DBStore) GetAll() ([]*models.Metrics, error) {
	return nil, nil
}

func (d *DBStore) SaveAll(metrics []*models.Metrics) error {
	panic("implement me")
}

func (d *DBStore) Close() error {
	d.pool.Close()
	return nil
}

func (d *DBStore) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}
