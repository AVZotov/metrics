package repository

import (
	"context"
	"errors"
	"time"
	
	"github.com/AVZotov/metrics/internal/config/db"
	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ PersistRepository = (*DBStore)(nil)

type DBStore struct {
	pool *pgxpool.Pool
	cfg  *db.Config
}

func NewDBStore(ctx context.Context, dsn string, cfg *db.Config) (*DBStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &DBStore{
		pool: pool,
		cfg:  cfg,
	}, nil
}

func (d *DBStore) Save(metrics *models.Metrics) error {
	ctx, cancel := context.WithTimeout(context.Background(), d.cfg.ConnectTimeout)
	defer cancel()
	query := `INSERT INTO metrics (id, mtype, delta, value)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id, mtype)
			DO UPDATE SET delta = EXCLUDED.delta, value = EXCLUDED.value`
	
	return witRetry(
		ctx,
		func() error {
			_, err := d.pool.Exec(ctx, query, metrics.ID, metrics.MType, metrics.Delta, metrics.Value)
			return err
		},
	)
}

func (d *DBStore) Get(id, mType string) (*models.Metrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.cfg.QueryTimeout)
	defer cancel()
	query := `SELECT id, mtype, delta, value, hash FROM metrics WHERE id = $1 AND mtype = $2`
	
	m := &models.Metrics{}
	var hash *string
	
	err := witRetry(
		ctx, func() error {
			return d.pool.QueryRow(ctx, query, id, mType).Scan(&m.ID, &m.MType, &m.Delta, &m.Value, &hash)
		},
	)
	
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	if hash != nil {
		m.Hash = *hash
	}
	return m, nil
}

func (d *DBStore) GetAll() ([]*models.Metrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.cfg.QueryTimeout)
	defer cancel()
	
	query := `SELECT id, mtype, delta, value, hash FROM metrics`
	
	var metrics []*models.Metrics
	err := witRetry(
		ctx, func() error {
			rows, err := d.pool.Query(ctx, query)
			if err != nil {
				return err
			}
			metrics, err = pgx.CollectRows(
				rows, func(row pgx.CollectableRow) (*models.Metrics, error) {
					m := &models.Metrics{}
					var hash *string
					if err := row.Scan(&m.ID, &m.MType, &m.Delta, &m.Value, &hash); err != nil {
						return nil, err
					}
					if hash != nil {
						m.Hash = *hash
					}
					return m, nil
				},
			)
			return err
		},
	)
	if err != nil {
		return nil, err
	}
	return metrics, nil
}

func (d *DBStore) SaveAll(metrics []*models.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), d.cfg.QueryTimeout)
	defer cancel()
	
	query := `INSERT INTO metrics (id, mtype, delta, value)
              VALUES ($1, $2, $3, $4)
              ON CONFLICT (id, mtype)
              DO UPDATE SET delta = EXCLUDED.delta, value = EXCLUDED.value`
	
	return witRetry(
		ctx, func() error {
			tx, err := d.pool.Begin(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback(ctx) }()
			
			batch := &pgx.Batch{}
			for _, m := range metrics {
				batch.Queue(query, m.ID, m.MType, m.Delta, m.Value)
			}
			
			if err := tx.SendBatch(ctx, batch).Close(); err != nil {
				return err
			}
			
			return tx.Commit(ctx)
		},
	)
}

func (d *DBStore) Close() error {
	d.pool.Close()
	return nil
}

func (d *DBStore) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

func witRetry(ctx context.Context, op func() error) error {
	delays := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}
	
	err := op()
	if err == nil || !isConnectionException(err) {
		return err
	}
	
	for _, delay := range delays {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		
		err = op()
		if err == nil {
			return nil
		}
		if !isConnectionException(err) {
			return err
		}
	}
	return err
}

func isConnectionException(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgerrcode.IsConnectionException(pgErr.Code)
}
