package repository

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/AVZotov/metrics"
)

// RunMigrations applies all embedded goose migrations against dsn. Returns
// an error if the connection fails or any migration fails to apply.
func RunMigrations(ctx context.Context, dsn string) (err error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	migrationsFS, err := fs.Sub(metrics.EmbedMigrations, "migrations")
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrationsFS)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	if _, err := provider.Up(ctx); err != nil {
		return err
	}

	return nil
}
