package errors

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNilDelta              = errors.New("metrics delta is nil")
	ErrNilValue              = errors.New("metrics value is nil")
	ErrUnknownMetricType     = errors.New("unknown metric type")
	ErrUnknownMetricValue    = errors.New("unknown metric value")
	ErrEmptyMetricType       = errors.New("empty metric type")
	ErrEmptyMetricName       = errors.New("empty metric name")
	ErrEmptyMetricValue      = errors.New("empty metric value")
	ErrNilMetric             = errors.New("metric is nil")
	ErrNotFound              = errors.New("not found")
	ErrInvalidValue          = errors.New("invalid value")
	ErrInvalidPollInterval   = errors.New("poll interval must be greater than 0")
	ErrInvalidReportInterval = errors.New("report interval must be greater than 0")
	ErrInvalidRateLimit      = errors.New("rate limit must be greater than 0")
	ErrUnknownFlags          = errors.New("unknown flag arguments")
	ErrRetriableStatus       = errors.New("retriable http status")
)

type RetryError struct {
	Succeeded bool
	Attempts  []error
}

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

type NetworkError struct {
	Err error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error: %v", e.Err)
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}
