package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
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

// TestReportWorker_QueuedJobAfterCancel_NotSent verifies that a worker
// returns without sending a job still queued when ctx is cancelled; the
// job's data is left for the final flush.
func TestReportWorker_QueuedJobAfterCancel_NotSent(t *testing.T) {
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

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "", netip.Addr{})
	require.NoError(t, err)
	a.Collect()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	jobs := make(chan []models.Metrics, 1)
	jobs <- a.Snapshot()

	done := make(chan struct{})
	go func() {
		reportWorker(ctx, context.Background(), jobs, a, zap.NewNop())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reportWorker did not return after ctx cancellation")
	}

	assert.Equal(t, int32(0), atomic.LoadInt32(&received))
	delta := findDelta(a.Snapshot(), "PollCount")
	require.NotNil(t, delta)
	assert.Equal(t, int64(1), *delta, "unsent job must stay unacked")
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

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "", netip.Addr{})
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

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "", netip.Addr{})
	require.NoError(t, err)

	flushPending(a, zap.NewNop())

	assert.Equal(t, int32(0), atomic.LoadInt32(&received))
}

// deltaServer is a test metrics server that sums the PollCount deltas of
// every batch it accepts, which is what the real server would add to its
// stored counter. Like the real updatesJSON handler, it decodes the whole
// body first and then processes it without regard to whether the client
// is still waiting. handle simulates that processing and may return a
// non-200 status to reject the batch. started is closed when the first
// request arrives.
type deltaServer struct {
	*httptest.Server
	started chan struct{}
	mu      sync.Mutex
	total   int64
}

func newDeltaServer(t *testing.T, handle func(n int) (status int)) *deltaServer {
	t.Helper()
	s := &deltaServer{started: make(chan struct{})}
	var calls int32
	s.Server = httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				n := int(atomic.AddInt32(&calls, 1))
				gz, err := gzip.NewReader(r.Body)
				require.NoError(t, err)
				var batch []models.Metrics
				require.NoError(t, json.NewDecoder(gz).Decode(&batch))
				if n == 1 {
					close(s.started)
				}

				if status := handle(n); status != http.StatusOK {
					w.WriteHeader(status)
					return
				}
				s.mu.Lock()
				if d := findDelta(batch, "PollCount"); d != nil {
					s.total += *d
				}
				s.mu.Unlock()
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	t.Cleanup(s.Close)
	return s
}

func (s *deltaServer) counterTotal() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

// shutdownScenario reproduces run()'s shutdown sequence while a worker is
// busy with the first report: once that report reaches the server, ctx is
// cancelled, stopWorkers runs with the given timeout, and only then does
// flushPending run. It returns how long stopWorkers took.
func shutdownScenario(t *testing.T, a *agent.Agent, server *deltaServer, timeout time.Duration) time.Duration {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sendCtx, cancelSends := context.WithCancel(context.Background())
	defer cancelSends()

	jobs := make(chan []models.Metrics, 1)
	jobs <- a.Snapshot()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reportWorker(ctx, sendCtx, jobs, a, zap.NewNop())
	}()

	select {
	case <-server.started:
	case <-time.After(time.Second):
		t.Fatal("worker never sent the first report")
	}
	cancel()

	start := time.Now()
	stopWorkers(&wg, cancelSends, timeout, zap.NewNop())
	elapsed := time.Since(start)

	flushPending(a, zap.NewNop())

	// A no-op when stopWorkers is correct. Otherwise it lets a worker that
	// outlived the flush finish its send before the caller closes the
	// server, so the resulting double count is observed.
	wg.Wait()
	return elapsed
}

// TestShutdown_InFlightSend_CounterCountedOnce covers a worker whose HTTP
// request is still being processed by the server when the signal arrives.
// The request must be allowed to finish rather than be aborted, and the
// flush must run after it, so the PollCount delta is counted exactly once.
func TestShutdown_InFlightSend_CounterCountedOnce(t *testing.T) {
	server := newDeltaServer(
		t, func(n int) int {
			if n == 1 {
				time.Sleep(300 * time.Millisecond)
			}
			return http.StatusOK
		},
	)

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "", netip.Addr{})
	require.NoError(t, err)
	a.Collect()

	shutdownScenario(t, a, server, 2*time.Second)

	// Close blocks until outstanding requests finish, so a request that
	// outlived the flush has been counted by the time we assert.
	server.Close()

	assert.Equal(t, int64(1), server.counterTotal())
}

// TestShutdown_InFlightSendPastTimeout_KnownDoubleCount documents the
// remaining edge case described on stopWorkers: when the server is still
// processing a request after the shutdown timeout, cancelling it doesn't
// stop the server from applying it, and the final flush then resends the
// same delta. If this test starts failing because the total is 1, the
// limitation has been fixed (e.g. server-side batch deduplication); update
// the test and the stopWorkers comment.
func TestShutdown_InFlightSendPastTimeout_KnownDoubleCount(t *testing.T) {
	server := newDeltaServer(
		t, func(n int) int {
			if n == 1 {
				time.Sleep(300 * time.Millisecond)
			}
			return http.StatusOK
		},
	)

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "", netip.Addr{})
	require.NoError(t, err)
	a.Collect()

	shutdownScenario(t, a, server, 50*time.Millisecond)
	server.Close()

	assert.Equal(t, int64(2), server.counterTotal(), "known limitation: delta counted twice")
}

// TestShutdown_RetryingSendPastTimeout_CounterCountedOnce covers a worker
// sleeping in SendWithRetry's backoff when the shutdown timeout expires.
// The backoff must be cancelled promptly, not resumed after the flush to
// resend the same delta.
func TestShutdown_RetryingSendPastTimeout_CounterCountedOnce(t *testing.T) {
	server := newDeltaServer(
		t, func(n int) int {
			if n == 1 {
				return http.StatusServiceUnavailable
			}
			return http.StatusOK
		},
	)

	a, err := agent.NewAgent(&http.Client{}, server.URL, "", "", netip.Addr{})
	require.NoError(t, err)
	a.Collect()

	elapsed := shutdownScenario(t, a, server, 50*time.Millisecond)
	server.Close()

	assert.Less(t, elapsed, 500*time.Millisecond, "backoff was not cancelled")
	assert.Equal(t, int64(1), server.counterTotal())
}

// TestStopWorkers_CancelsThenWaitsForWorkers verifies stopWorkers cancels
// sends once the timeout expires and does not return until the workers
// have actually exited.
func TestStopWorkers_CancelsThenWaitsForWorkers(t *testing.T) {
	sendCtx, cancelSends := context.WithCancel(context.Background())
	defer cancelSends()

	var workerExited atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-sendCtx.Done()
		time.Sleep(50 * time.Millisecond)
		workerExited.Store(true)
	}()

	stopWorkers(&wg, cancelSends, 20*time.Millisecond, zap.NewNop())

	assert.Error(t, sendCtx.Err(), "sends should be cancelled after timeout")
	assert.True(t, workerExited.Load(), "stopWorkers returned before the worker exited")
}

func TestStopWorkers_CleanStopDoesNotCancel(t *testing.T) {
	sendCtx, cancelSends := context.WithCancel(context.Background())
	defer cancelSends()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()

	stopWorkers(&wg, cancelSends, time.Second, zap.NewNop())

	assert.NoError(t, sendCtx.Err())
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
