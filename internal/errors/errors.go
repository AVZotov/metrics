package errors

import "errors"

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
	ErrUnknownFlags          = errors.New("unknown flag arguments")
	ErrEmptyFilePath         = errors.New("path cannot be empty")
	ErrEmptyFileName         = errors.New("path must include a file name")
)
