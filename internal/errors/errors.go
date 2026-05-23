package errors

import "errors"

var ErrNilDelta = errors.New("metrics delta is nil")
var ErrNilValue = errors.New("metrics value is nil")
var ErrUnknownMetricType = errors.New("unknown metric type")
var ErrUnknownMetricValue = errors.New("unknown metric value")
var ErrEmptyMetricType = errors.New("empty metric type")
var ErrEmptyMetricName = errors.New("empty metric name")
var ErrEmptyMetricValue = errors.New("empty metric value")
var ErrNilMetric = errors.New("metric is nil")
var ErrNotFound = errors.New("not found")
var ErrInvalidValue = errors.New("invalid value")
