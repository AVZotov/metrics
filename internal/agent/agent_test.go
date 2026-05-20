package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
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
	err := a.Report()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(gMetrics)+len(cMetrics), counter)
}

func TestAgent_Report_Metrics_ContentType(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				cType := r.Header.Get("Content-Type")
				assert.Equal(t, "text/plain", cType)
			},
		),
	)
	defer server.Close()

	a := NewAgent(&http.Client{}, server.URL)
	a.Collect()
	err := a.Report()
	if err != nil {
		t.Fatal(err)
	}
}
