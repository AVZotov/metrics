package service

import (
	"strconv"
	"testing"

	models "github.com/AVZotov/metrics/internal/model"
)

func makeBenchMetrics(n int) []models.Metrics {
	metrics := make([]models.Metrics, n)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			d := int64(i)
			metrics[i] = models.Metrics{ID: "counter", MType: models.Counter, Delta: &d}
		} else {
			v := float64(i)
			metrics[i] = models.Metrics{ID: "gauge", MType: models.Gauge, Value: &v}
		}
	}
	return metrics
}

func BenchmarkUpdateMetrics(b *testing.B) {
	sizes := []int{1, 10, 100, 1000}
	for _, n := range sizes {
		b.Run(
			strconv.Itoa(n), func(b *testing.B) {
				metrics := makeBenchMetrics(n)
				svc := NewMetricsService(&mockRepo{}, newTestNotifier())

				b.ReportAllocs()
				for b.Loop() {
					_ = svc.UpdateMetrics(metrics, "")
				}
			},
		)
	}
}
