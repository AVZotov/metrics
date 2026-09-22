package agent

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	models "github.com/AVZotov/metrics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestPublicKeyPEM generates a throwaway RSA key pair and writes its
// public half to a PKIX PEM file in t's temp dir, returning the file path.
func writeTestPublicKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)

	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	path := filepath.Join(t.TempDir(), "pubkey.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))

	return path
}

func TestAgent_Collect_Check_Count(t *testing.T) {
	want := int64(1)
	a, err := NewAgent(&http.Client{}, "", "", "")
	require.NoError(t, err)
	a.Collect()
	got := a.counter["PollCount"]
	assert.Equal(t, want, got)
}

func TestAgent_Collect_Check_Gauge(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "", "", "")
	require.NoError(t, err)
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

	a, err := NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)
	a.Collect()
	err = a.Report(context.Background())
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

	a, err := NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)
	a.Collect()
	err = a.Report(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewAgent(t *testing.T) {
	client := &http.Client{}
	a, err := NewAgent(client, "http://localhost:8080", "", "")
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:8080", a.baseURL)
	assert.NotNil(t, a.gauge)
	assert.NotNil(t, a.counter)
	assert.Equal(t, client, a.client)
	assert.Nil(t, a.pubKey)
}

func TestNewAgent_EmptyCryptoKeyPath_NoPubKey(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "http://localhost:8080", "", "")
	require.NoError(t, err)
	assert.Nil(t, a.pubKey)
}

func TestNewAgent_ValidCryptoKeyPath_SetsPubKey(t *testing.T) {
	path := writeTestPublicKeyPEM(t)

	a, err := NewAgent(&http.Client{}, "http://localhost:8080", "", path)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.NotNil(t, a.pubKey)
}

func TestNewAgent_InvalidCryptoKeyPath_ReturnsError(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "http://localhost:8080", "", "/nonexistent/path/to/key.pem")
	assert.Error(t, err)
	assert.Nil(t, a)
}

func TestAgent_Collect_PollCount_Accumulates(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "", "", "")
	require.NoError(t, err)
	a.Collect()
	a.Collect()
	a.Collect()
	assert.Equal(t, int64(3), a.counter["PollCount"])
}

func TestAgent_Report_ContentEncoding(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "gzip", r.Header.Get("Content-Encoding"))
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer server.Close()

	a, err := NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)
	a.Collect()
	require.NoError(t, a.Report(context.Background()))
}

func TestAgent_Report_URL(t *testing.T) {
	var gotPaths []string
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				gotPaths = append(gotPaths, r.URL.Path)
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer server.Close()

	a, err := NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)
	a.Collect()
	require.NoError(t, a.Report(context.Background()))

	// All metrics are sent as a single batch to /updates/
	require.Len(t, gotPaths, 1)
	assert.Equal(t, "/updates/", gotPaths[0])
}

func TestAgent_Report_Body_Gauge(t *testing.T) {
	var received []models.Metrics
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				gz, err := gzip.NewReader(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer func() { _ = gz.Close() }()
				var batch []models.Metrics
				if err := json.NewDecoder(gz).Decode(&batch); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				received = append(received, batch...)
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer server.Close()

	a, err := NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)
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
	a, err := NewAgent(&http.Client{}, "http://127.0.0.1:1", "", "")
	require.NoError(t, err)
	a.Collect()
	err = a.Report(context.Background())
	assert.Error(t, err)
}

func TestAgent_SendMetricJSON_InvalidGaugeValue(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "http://localhost:8080", "", "")
	require.NoError(t, err)
	err = a.sendMetricJSON(models.Gauge, "TestMetric", "notanumber")
	assert.Error(t, err)
}

func TestAgent_SendMetricJSON_InvalidCounterValue(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "http://localhost:8080", "", "")
	require.NoError(t, err)
	err = a.sendMetricJSON(models.Counter, "PollCount", "notanumber")
	assert.Error(t, err)
}

func TestAgent_SendMetricJSON_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		),
	)
	defer server.Close()

	a, err := NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)
	err = a.sendMetricJSON(models.Gauge, "Alloc", "1.5")
	assert.Error(t, err)
}

func TestAgent_SendMetric(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/update/gauge/Alloc/42.5", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer server.Close()

	a, err := NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)
	err = a.sendMetric("gauge", "Alloc", "42.5")
	require.NoError(t, err)
}

func TestAgent_AckSent_SubtractsDelta(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "", "", "")
	require.NoError(t, err)
	a.counter["PollCount"] = 10

	delta := int64(4)
	a.AckSent([]models.Metrics{{ID: "PollCount", MType: models.Counter, Delta: &delta}})

	assert.Equal(t, int64(6), a.counter["PollCount"])
}

func TestAgent_AckSent_SkipsNonCounterMetrics(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "", "", "")
	require.NoError(t, err)
	a.counter["PollCount"] = 10
	a.gauge["Alloc"] = 42

	value := 99.0
	a.AckSent([]models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &value}})

	assert.Equal(t, int64(10), a.counter["PollCount"])
	assert.Equal(t, 42.0, a.gauge["Alloc"])
}

func TestAgent_AckSent_SkipsNilDelta(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "", "", "")
	require.NoError(t, err)
	a.counter["PollCount"] = 10

	assert.NotPanics(t, func() {
		a.AckSent([]models.Metrics{{ID: "PollCount", MType: models.Counter, Delta: nil}})
	})
	assert.Equal(t, int64(10), a.counter["PollCount"])
}

// TestAgent_AckSent_PreservesIncrementsDuringRoundTrip covers the bug AckSent
// fixed: acking a sent snapshot must subtract exactly what was sent, not reset
// the counter to zero, so increments collected while the report was in flight
// survive.
func TestAgent_AckSent_PreservesIncrementsDuringRoundTrip(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "", "", "")
	require.NoError(t, err)
	a.Collect() // PollCount = 1
	sent := a.Snapshot()

	a.Collect() // increment that happens during the "network round-trip"; PollCount = 2

	a.AckSent(sent)

	assert.Equal(t, int64(1), a.counter["PollCount"])
}

func TestAgent_AckSent_ConcurrentWithCollect(t *testing.T) {
	a, err := NewAgent(&http.Client{}, "", "", "")
	require.NoError(t, err)
	const n = 500

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			a.Collect()
		}
	}()
	go func() {
		defer wg.Done()
		zero := int64(0)
		for i := 0; i < n; i++ {
			a.AckSent([]models.Metrics{{ID: "PollCount", MType: models.Counter, Delta: &zero}})
		}
	}()
	wg.Wait()

	assert.Equal(t, int64(n), a.counter["PollCount"])
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
			a, err := NewAgent(&http.Client{}, server.URL, "", "")
			require.NoError(t, err)
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
