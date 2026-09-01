// Package errors defines sentinel errors and custom error types shared
// across the agent and server, used to classify failures (validation,
// retriable network/status errors, etc.) without string matching.
package errors

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNilDelta means a counter metric has no Delta set.
	ErrNilDelta = errors.New("metrics delta is nil")
	// ErrNilValue means a gauge metric has no Value set.
	ErrNilValue = errors.New("metrics value is nil")
	// ErrUnknownMetricType means the metric type isn't "counter" or "gauge".
	ErrUnknownMetricType = errors.New("unknown metric type")
	// ErrUnknownMetricValue means the metric value couldn't be parsed as the expected type.
	ErrUnknownMetricValue = errors.New("unknown metric value")
	// ErrEmptyMetricType means the metric type field was left blank.
	ErrEmptyMetricType = errors.New("empty metric type")
	// ErrEmptyMetricName means the metric name field was left blank.
	ErrEmptyMetricName = errors.New("empty metric name")
	// ErrEmptyMetricValue means the metric value field was left blank.
	ErrEmptyMetricValue = errors.New("empty metric value")
	// ErrNilMetric means a nil metric was passed where one was required.
	ErrNilMetric = errors.New("metric is nil")
	// ErrNotFound means the requested metric doesn't exist in the store.
	ErrNotFound = errors.New("not found")
	// ErrInvalidValue means a value couldn't be parsed into its expected form.
	ErrInvalidValue = errors.New("invalid value")
	// ErrInvalidPollInterval means the configured poll interval is 0 or negative.
	ErrInvalidPollInterval = errors.New("poll interval must be greater than 0")
	// ErrInvalidReportInterval means the configured report interval is 0 or negative.
	ErrInvalidReportInterval = errors.New("report interval must be greater than 0")
	// ErrInvalidRateLimit means the configured rate limit is 0 or negative.
	ErrInvalidRateLimit = errors.New("rate limit must be greater than 0")
	// ErrUnknownFlags means unrecognized command-line arguments were passed.
	ErrUnknownFlags = errors.New("unknown flag arguments")
	// ErrRetriableStatus wraps an HTTP response status that's worth retrying (5xx).
	ErrRetriableStatus = errors.New("retriable http status")
)

// RetryError reports the outcome of a retried operation, including every
// attempt's error, so callers can inspect the full retry history.
type RetryError struct {
	Succeeded bool
	Attempts  []error
}

// Error formats the retry outcome and every attempt's error into one message.
func (e *RetryError) Error() string {
	msgs := make([]string, 0, len(e.Attempts))
	for i, err := range e.Attempts {
		msgs = append(msgs, fmt.Sprintf("attempt %d: %v", i+1, err))
	}
	status := "failed"
	if e.Succeeded {
		status = "succeeded"
	}
	return fmt.Sprintf("retry %s after %d attempts: %s", status, len(e.Attempts), strings.Join(msgs, "; "))
}

// NetworkError wraps a transport-level error (connection refused, timeout,
// etc.) so callers can tell it apart from other error types and retry it.
type NetworkError struct {
	Err error
}

// Error returns the wrapped error prefixed with "network error: ".
func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error: %v", e.Err)
}

// Unwrap returns the underlying error for use with errors.Is/errors.As.
func (e *NetworkError) Unwrap() error {
	return e.Err
}
