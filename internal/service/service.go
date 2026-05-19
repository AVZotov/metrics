package service

type Service interface {
	UpdateMetric(metricType, name, value string) error
}
