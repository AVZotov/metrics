package agent

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"runtime"
	"strconv"
)

type Agent struct {
	client  *http.Client
	baseURL string
	gauge   map[string]float64
	counter map[string]int64
}

func NewAgent(client *http.Client, baseURL string) *Agent {
	gauge := make(map[string]float64, len(gMetrics))
	counter := make(map[string]int64, len(cMetrics))
	return &Agent{
		client:  client,
		baseURL: baseURL,
		gauge:   gauge,
		counter: counter,
	}
}

func (a *Agent) Collect() {
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

	a.counter["PollCount"] += 1
}

func (a *Agent) Report() error {
	for k, v := range a.gauge {
		sv := strconv.FormatFloat(v, 'f', -1, 64)
		if err := a.sendMetric("gauge", k, sv); err != nil {
			return err
		}
	}

	for k, v := range a.counter {
		sv := strconv.FormatInt(v, 10)
		if err := a.sendMetric("counter", k, sv); err != nil {
			return err
		}
	}
	return nil
}

func (a *Agent) sendMetric(metricType, name, value string) error {
	url := fmt.Sprintf("%s/update/%s/%s/%s", a.baseURL, metricType, name, value)
	resp, err := a.client.Post(url, "text/plain", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
