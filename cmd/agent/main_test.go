package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AVZotov/metrics/internal/agent"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func findDelta(metrics []models.Metrics, id string) *int64 {
	for _, m := range metrics {
		if m.ID == id {
			return m.Delta
		}
	}
	return nil
}

// TestReportWorker_DrainsQueuedJobsAfterCancel verifies that a job already
// queued in the jobs channel still gets sent even though ctx was cancelled
// before the worker picked it up, since sendMetrics falls back to a fresh
// shutdown-bounded context instead of the cancelled one.
func TestReportWorker_DrainsQueuedJobsAfterCancel(t *testing.T) {
	var received int32
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&received, 1)
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer server.Close()

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	jobs := make(chan []models.Metrics, 1)
	value := 1.0
	jobs <- []models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &value}}
	close(jobs)

	done := make(chan struct{})
	go func() {
		reportWorker(ctx, jobs, a, zap.NewNop())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reportWorker did not exit after jobs channel closed")
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&received))
}

// TestReportLoop_ClosesJobsOnCtxDone verifies reportLoop is the one that
// closes the jobs channel once ctx is cancelled, which is what lets
// reportWorker's range loop terminate.
func TestReportLoop_ClosesJobsOnCtxDone(t *testing.T) {
	a, err := agent.NewAgent(&http.Client{}, "http://127.0.0.1:0", "", "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	jobs := make(chan []models.Metrics, 1)

	done := make(chan struct{})
	go func() {
		reportLoop(ctx, a, jobs, time.Hour)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reportLoop did not return after ctx cancellation")
	}

	_, ok := <-jobs
	assert.False(t, ok, "jobs channel should be closed by reportLoop")
}

// TestFlushPending_SendsPendingSnapshotAndAcks verifies the final shutdown
// flush sends whatever metrics are currently held by the agent and, on
// success, acknowledges sent counters the same way a normal report would.
func TestFlushPending_SendsPendingSnapshotAndAcks(t *testing.T) {
	var received int32
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&received, 1)
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer server.Close()

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)
	a.Collect()

	flushPending(a, zap.NewNop())

	require.Equal(t, int32(1), atomic.LoadInt32(&received))

	delta := findDelta(a.Snapshot(), "PollCount")
	require.NotNil(t, delta)
	assert.Equal(t, int64(0), *delta)
}

// TestFlushPending_NoPendingMetrics_NoRequest verifies flushPending is a
// no-op, sending nothing, when the agent never collected anything.
func TestFlushPending_NoPendingMetrics_NoRequest(t *testing.T) {
	var received int32
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&received, 1)
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer server.Close()

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "")
	require.NoError(t, err)

	flushPending(a, zap.NewNop())

	assert.Equal(t, int32(0), atomic.LoadInt32(&received))
}

func TestWaitWithTimeout_ReturnsTrueWhenWorkFinishes(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()

	assert.True(t, waitWithTimeout(&wg, time.Second))
}

func TestWaitWithTimeout_ReturnsFalseOnTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	assert.False(t, waitWithTimeout(&wg, 20*time.Millisecond))

	wg.Done()
}
