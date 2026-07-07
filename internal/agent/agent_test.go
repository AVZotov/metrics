package agent

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	models "github.com/AVZotov/metrics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent_Collect_Check_Count(t *testing.T) {
	want := int64(1)
	a := NewAgent(&http.Client{}, "")
	a.Collect()
	got := a.counter["PollCount"]
	assert.Equal(t, want, got)
}

func TestAgent_Collect_Check_Gauge(t *testing.T) {
	a := NewAgent(&http.Client{}, "")
	a.Collect()
	for _, k := range gMetrics {
		assert.Contains(t, a.gauge, k, "metric %s not found in gauge", k)
	}
}

func TestAgent_Report_Metrics_Count(t *testing.T) {
	counter := 0
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				counter++
			},
		),
	)
	defer server.Close()

	a := NewAgent(&http.Client{}, server.URL)
	a.Collect()
	err := a.Report(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// All metrics are sent in a single batch request to /updates/
	assert.Equal(t, 1, counter)
}

func TestAgent_Report_Metrics_ContentType(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				cType := r.Header.Get("Content-Type")
				assert.Equal(t, "application/json", cType)
			},
		),
	)
	defer server.Close()

	a := NewAgent(&http.Client{}, server.URL)
	a.Collect()
	err := a.Report(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewAgent(t *testing.T) {
	client := &http.Client{}
	a := NewAgent(client, "http://localhost:8080")

	assert.Equal(t, "http://localhost:8080", a.baseURL)
	assert.NotNil(t, a.gauge)
	assert.NotNil(t, a.counter)
	assert.Equal(t, client, a.client)
}

func TestAgent_Collect_PollCount_Accumulates(t *testing.T) {
	a := NewAgent(&http.Client{}, "")
	a.Collect()
	a.Collect()
	a.Collect()
	assert.Equal(t, int64(3), a.counter["PollCount"])
}

func TestAgent_Report_ContentEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "gzip", r.Header.Get("Content-Encoding"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := NewAgent(&http.Client{}, server.URL)
	a.Collect()
	require.NoError(t, a.Report(context.Background()))
}

func TestAgent_Report_URL(t *testing.T) {
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := NewAgent(&http.Client{}, server.URL)
	a.Collect()
	require.NoError(t, a.Report(context.Background()))

	// All metrics are sent as a single batch to /updates/
	require.Len(t, gotPaths, 1)
	assert.Equal(t, "/updates/", gotPaths[0])
}

func TestAgent_Report_Body_Gauge(t *testing.T) {
	var received []models.Metrics
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer gz.Close()
		var batch []models.Metrics
		if err := json.NewDecoder(gz).Decode(&batch); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received = append(received, batch...)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := NewAgent(&http.Client{}, server.URL)
	a.Collect()
	require.NoError(t, a.Report(context.Background()))

	gaugeCount, counterCount := 0, 0
	for _, m := range received {
		switch m.MType {
		case models.Gauge:
			assert.NotNil(t, m.Value, "gauge %s should have Value set", m.ID)
			assert.Nil(t, m.Delta, "gauge %s should not have Delta", m.ID)
			gaugeCount++
		case models.Counter:
			assert.NotNil(t, m.Delta, "counter %s should have Delta set", m.ID)
			assert.Nil(t, m.Value, "counter %s should not have Value", m.ID)
			counterCount++
		}
	}
	assert.Equal(t, len(gMetrics), gaugeCount)
	assert.Equal(t, len(cMetrics), counterCount)
}

func TestAgent_Report_Error_On_Unreachable_Server(t *testing.T) {
	a := NewAgent(&http.Client{}, "http://127.0.0.1:1")
	a.Collect()
	err := a.Report(context.Background())
	assert.Error(t, err)
}

func TestAgent_SendMetricJSON_InvalidGaugeValue(t *testing.T) {
	a := NewAgent(&http.Client{}, "http://localhost:8080")
	err := a.sendMetricJSON(models.Gauge, "TestMetric", "notanumber")
	assert.Error(t, err)
}

func TestAgent_SendMetricJSON_InvalidCounterValue(t *testing.T) {
	a := NewAgent(&http.Client{}, "http://localhost:8080")
	err := a.sendMetricJSON(models.Counter, "PollCount", "notanumber")
	assert.Error(t, err)
}

func TestAgent_SendMetricJSON_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := NewAgent(&http.Client{}, server.URL)
	err := a.sendMetricJSON(models.Gauge, "Alloc", "1.5")
	assert.Error(t, err)
}

func TestAgent_SendMetric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/update/gauge/Alloc/42.5", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := NewAgent(&http.Client{}, server.URL)
	err := a.sendMetric("gauge", "Alloc", "42.5")
	require.NoError(t, err)
}

func TestAgent_ConcurrentCollectReport(t *testing.T) {
	const requests = 1000
	tests := []struct {
		name string
		want int64
	}{
		{
			name: "concurrent Collect() and Report() N times no error",
			want: requests,
		},
	}

	for _, tt := range tests {
		var wg sync.WaitGroup
		var server *httptest.Server
		func() {
			server = httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
					},
				),
			)
			defer server.Close()
			a := NewAgent(&http.Client{}, server.URL)
			t.Run(
				tt.name, func(t *testing.T) {
					for i := 0; i < requests; i++ {
						wg.Add(2)
						go func() {
							defer wg.Done()
							a.Collect()
						}()
						go func() {
							defer wg.Done()
							err := a.Report(context.Background())
							if err != nil {
								assert.NoError(t, err)
							}
						}()
						wg.Wait()
					}
				},
			)
			assert.Equal(t, tt.want, a.counter["PollCount"])
		}()
	}
}
