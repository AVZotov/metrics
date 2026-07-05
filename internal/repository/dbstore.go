package repository

import (
	"context"
	"errors"
	"fmt"
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
	ctx, cancel := context.WithTimeout(context.Background(), d.cfg.QueryTimeout)
	defer cancel()
	query := `INSERT INTO metrics (id, mtype, delta, value)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id, mtype)
			DO UPDATE SET delta = EXCLUDED.delta, value = EXCLUDED.value`

	return withRetry(
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

	err := withRetry(
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
	err := withRetry(
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

//Александр привет!
// Ты давал комментарий к коду
//"а также counter-метрики перезаписываются вместо накопления при upsert"
// на всякий случай еще добавил комментарий к самим функциям
// Но возможно я не совсем точно понял сам комментарий
//я использовал MemStore как единственный источник финальных данных
// и хотел уйти от дублирования логики накопления

// SaveAll delta is overwritten, not summed: MemStore already accumulates the
// total before Dump() is called, so this upsert just persists the
// current snapshot. Summing here would double-count.
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

	return withRetry(
		ctx, func() error {
			tx, err := d.pool.Begin(ctx)
			if err != nil {
				return err
			}

			batch := &pgx.Batch{}
			for _, m := range metrics {
				batch.Queue(query, m.ID, m.MType, m.Delta, m.Value)
			}
			br := tx.SendBatch(ctx, batch)
			for i, m := range metrics {
				if _, err := br.Exec(); err != nil {
					_ = br.Close()
					_ = tx.Rollback(ctx)
					return fmt.Errorf("batch item %d (metric %s): %w", i, m.ID, err)
				}
			}
			if err := br.Close(); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			if err := tx.Commit(ctx); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			return nil
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

func withRetry(ctx context.Context, op func() error) error {
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
