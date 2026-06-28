package repository

import (
	"context"
	"database/sql"
	"io/fs"
	
	"github.com/pressly/goose/v3"
	
	_ "github.com/jackc/pgx/v5/stdlib"
	
	"github.com/AVZotov/metrics"
)

func RunMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	
	migrationsFS, err := fs.Sub(metrics.EmbedMigrations, "migrations")
	if err != nil {
		return err
	}
	
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrationsFS)
	if err != nil {
		return err
	}
	defer provider.Close()
	
	if _, err := provider.Up(ctx); err != nil {
		return err
	}
	
	return nil
}
