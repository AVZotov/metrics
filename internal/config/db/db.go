// Package db holds configuration shared by database-backed repository code.
package db

import "time"

// Config holds timeouts used when talking to the database.
type Config struct {
	ConnectTimeout time.Duration
	QueryTimeout   time.Duration
}
