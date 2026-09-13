// Package metrics is the module root, used here to hold the embedded SQL
// migration files so they can be shipped inside the compiled binary
// instead of read from disk at runtime.
package metrics

import "embed"

// EmbedMigrations holds the goose SQL migration files embedded into the binary.
//
//go:embed migrations/*.sql
var EmbedMigrations embed.FS
