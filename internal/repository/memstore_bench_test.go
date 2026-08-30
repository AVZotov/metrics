package repository

import (
	"strconv"
	"testing"

	models "github.com/AVZotov/metrics/internal/model"
)

func makeBenchMemStoreMetrics(n int) []*models.Metrics {
	metrics := make([]*models.Metrics, n)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			metrics[i] = &models.Metrics{ID: "counter" + strconv.Itoa(i), MType: models.Counter, Delta: new(int64(i))}
		} else {
			metrics[i] = &models.Metrics{ID: "gauge" + strconv.Itoa(i), MType: models.Gauge, Value: new(float64(i))}
		}
	}
	return metrics
}

func BenchmarkMemStore_Save(b *testing.B) {
	sizes := []int{1, 10, 100, 1000}
	for _, n := range sizes {
		b.Run(
			strconv.Itoa(n), func(b *testing.B) {
				metrics := makeBenchMemStoreMetrics(n)
				s := NewMemStore()

				b.ReportAllocs()
				for b.Loop() {
					for _, m := range metrics {
						_ = s.Save(m)
					}
				}
			},
		)
	}
}

func BenchmarkMemStore_SaveAll(b *testing.B) {
	sizes := []int{1, 10, 100, 1000}
	for _, n := range sizes {
		b.Run(
			strconv.Itoa(n), func(b *testing.B) {
				metrics := makeBenchMemStoreMetrics(n)
				s := NewMemStore()

				b.ReportAllocs()
				for b.Loop() {
					_ = s.SaveAll(metrics)
				}
			},
		)
	}
}

func BenchmarkMemStore_GetAll(b *testing.B) {
	sizes := []int{1, 10, 100, 1000}
	for _, n := range sizes {
		b.Run(
			strconv.Itoa(n), func(b *testing.B) {
				s := NewMemStore()
				metrics := makeBenchMemStoreMetrics(n)
				_ = s.SaveAll(metrics)

				b.ReportAllocs()
				for b.Loop() {
					_, _ = s.GetAll()
				}
			},
		)
	}
}
