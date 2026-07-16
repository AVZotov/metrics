package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	apperrors "github.com/AVZotov/metrics/internal/errors"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/AVZotov/metrics/internal/sign"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type Agent struct {
	mu      sync.Mutex
	client  *http.Client
	baseURL string
	gauge   map[string]float64
	counter map[string]int64
	key     string
}

func NewAgent(client *http.Client, baseURL string, key string) *Agent {
	gauge := make(map[string]float64, len(gMetrics))
	counter := make(map[string]int64, len(cMetrics))
	return &Agent{
		client:  client,
		baseURL: baseURL,
		gauge:   gauge,
		counter: counter,
		key:     key,
	}
}

func (a *Agent) Collect() {
	a.mu.Lock()
	defer a.mu.Unlock()

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	a.gauge["Alloc"] = float64(stats.Alloc)
	a.gauge["BuckHashSys"] = float64(stats.BuckHashSys)
	a.gauge["Frees"] = float64(stats.Frees)
	a.gauge["GCCPUFraction"] = stats.GCCPUFraction
	a.gauge["GCSys"] = float64(stats.GCSys)
	a.gauge["HeapAlloc"] = float64(stats.HeapAlloc)
	a.gauge["HeapIdle"] = float64(stats.HeapIdle)
	a.gauge["HeapInuse"] = float64(stats.HeapInuse)
	a.gauge["HeapObjects"] = float64(stats.HeapObjects)
	a.gauge["HeapReleased"] = float64(stats.HeapReleased)
	a.gauge["HeapSys"] = float64(stats.HeapSys)
	a.gauge["LastGC"] = float64(stats.LastGC)
	a.gauge["Lookups"] = float64(stats.Lookups)
	a.gauge["MCacheInuse"] = float64(stats.MCacheInuse)
	a.gauge["MCacheSys"] = float64(stats.MCacheSys)
	a.gauge["MSpanInuse"] = float64(stats.MSpanInuse)
	a.gauge["MSpanSys"] = float64(stats.MSpanSys)
	a.gauge["Mallocs"] = float64(stats.Mallocs)
	a.gauge["NextGC"] = float64(stats.NextGC)
	a.gauge["NumForcedGC"] = float64(stats.NumForcedGC)
	a.gauge["NumGC"] = float64(stats.NumGC)
	a.gauge["OtherSys"] = float64(stats.OtherSys)
	a.gauge["PauseTotalNs"] = float64(stats.PauseTotalNs)
	a.gauge["StackInuse"] = float64(stats.StackInuse)
	a.gauge["StackSys"] = float64(stats.StackSys)
	a.gauge["Sys"] = float64(stats.Sys)
	a.gauge["TotalAlloc"] = float64(stats.TotalAlloc)
	a.gauge["RandomValue"] = rand.Float64()

	a.counter["PollCount"]++
}

// CollectGopsutil polls system
func (a *Agent) CollectGopsutil() error {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("could not read virtual memory stats: %w", err)
	}
	percentages, err := cpu.Percent(0, true)
	if err != nil {
		return fmt.Errorf("could not read cpu utilization: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.gauge["TotalMemory"] = float64(vm.Total)
	a.gauge["FreeMemory"] = float64(vm.Free)
	for i, p := range percentages {
		a.gauge[fmt.Sprintf("CPUutilization%d", i+1)] = p
	}

	return nil
}

// Report collects the current snapshot and sends it synchronously.
func (a *Agent) Report(ctx context.Context) error {
	return a.SendWithRetry(ctx, a.Snapshot())
}

func (a *Agent) Snapshot() []models.Metrics {
	a.mu.Lock()
	defer a.mu.Unlock()
	return toMetricsSlice(a.gauge, a.counter)
}

func (a *Agent) SendWithRetry(ctx context.Context, metrics []models.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}
	if err := a.sendMetricsJSON(metrics); err != nil {
		return a.retryReport(ctx, metrics, err)
	}

	return nil
}

// Unused functions for back compatibility
func (a *Agent) sendMetric(metricType, name, value string) error {
	url := fmt.Sprintf("%s/update/%s/%s/%s", a.baseURL, metricType, name, value)
	resp, err := a.client.Post(url, "text/plain", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Unused functions for back compatibility
func (a *Agent) sendMetricJSON(metricType, name, value string) error {
	url := fmt.Sprintf("%s/update", a.baseURL)
	m := models.Metrics{
		ID:    name,
		MType: metricType,
	}
	switch metricType {
	case models.Gauge:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		m.Value = &v
	case models.Counter:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		m.Delta = &v
	}
	buf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(buf)
	if err := json.NewEncoder(gz).Encode(m); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := a.client.Do(req)
	if err != nil {
		return &apperrors.NetworkError{Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return httpStatusError(resp.StatusCode)
	}
	return nil
}

func (a *Agent) sendMetricsJSON(metrics []models.Metrics) error {
	url := fmt.Sprintf("%s/updates/", a.baseURL)
	buf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(buf)
	if err := json.NewEncoder(gz).Encode(metrics); err != nil {
		return fmt.Errorf("could not encode metrics: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("could not close gzip writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, buf)
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	if a.key != "" {
		signature := sign.Sign(buf.Bytes(), a.key)
		req.Header.Set("HashSHA256", signature)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return &apperrors.NetworkError{Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return httpStatusError(resp.StatusCode)
	}
	return nil
}

func (a *Agent) retryReport(ctx context.Context, metrics []models.Metrics, firstErr error) error {
	if !isRetriable(firstErr) {
		return &apperrors.RetryError{Succeeded: false, Attempts: []error{firstErr}}
	}

	delays := []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}
	attempts := []error{firstErr}

	for _, delay := range delays {
		select {
		case <-ctx.Done():
			attempts = append(attempts, ctx.Err())
			return &apperrors.RetryError{Succeeded: false, Attempts: attempts}
		case <-time.After(delay):
		}

		err := a.sendMetricsJSON(metrics)
		if err == nil {
			return &apperrors.RetryError{Succeeded: true, Attempts: attempts}
		}
		attempts = append(attempts, err)
		if !isRetriable(err) {
			return &apperrors.RetryError{Succeeded: false, Attempts: attempts}
		}
	}

	return &apperrors.RetryError{Succeeded: false, Attempts: attempts}
}

func toMetricsSlice(gauge map[string]float64, counter map[string]int64) []models.Metrics {
	metrics := make([]models.Metrics, 0, len(gauge)+len(counter))
	for k, v := range gauge {
		v := v
		metrics = append(metrics, models.Metrics{ID: k, MType: models.Gauge, Value: &v})
	}
	for k, v := range counter {
		v := v
		metrics = append(metrics, models.Metrics{ID: k, MType: models.Counter, Delta: &v})
	}
	return metrics
}

func httpStatusError(statusCode int) error {
	if statusCode >= 500 {
		return fmt.Errorf("%w: unexpected response status code %d", apperrors.ErrRetriableStatus, statusCode)
	}
	return fmt.Errorf("unexpected response status code %d", statusCode)
}

func isRetriable(err error) bool {
	if _, ok := errors.AsType[*apperrors.NetworkError](err); ok {
		return true
	}

	return errors.Is(err, apperrors.ErrRetriableStatus)
}
