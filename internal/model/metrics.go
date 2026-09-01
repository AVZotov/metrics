// Package models defines the Metrics data model shared across the agent,
// server, repository, and service layers.
package models

const (
	// Counter identifies a metric that accumulates (delta values only).
	Counter = "counter"
	// Gauge identifies a metric that holds a single point-in-time value.
	Gauge = "gauge"
)

// Metrics represents a single metric, either a counter or a gauge.
type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
	Hash  string   `json:"hash,omitempty"`
}
