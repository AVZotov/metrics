package audit

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/AVZotov/metrics/internal/config"
)

type fakeObserver struct {
	name     string
	err      error
	calls    int
	received []Event
}

func (f *fakeObserver) Notify(e Event) error {
	f.calls++
	f.received = append(f.received, e)
	return f.err
}

func (f *fakeObserver) Name() string { return f.name }

// fakeSlowCloserObserver is an Observer + io.Closer with a configurable
// Notify delay, used to exercise Notifier's async fan-out and Shutdown paths.
type fakeSlowCloserObserver struct {
	name      string
	delay     time.Duration
	notifyErr error
	closeErr  error

	calls       atomic.Int32
	closeCalled atomic.Bool
}

func (f *fakeSlowCloserObserver) Notify(_ Event) error {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.calls.Add(1)
	return f.notifyErr
}

func (f *fakeSlowCloserObserver) Name() string { return f.name }

func (f *fakeSlowCloserObserver) Close() error {
	f.closeCalled.Store(true)
	return f.closeErr
}

func TestNewNotifier_EmptyConfig(t *testing.T) {
	n := NewNotifier(&config.AuditConfig{}, zap.NewNop())
	require.NotNil(t, n)
	assert.Empty(t, n.observers)

	assert.NotPanics(t, func() {
		n.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))
	})
}

func TestNewNotifier_FileOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	n := NewNotifier(&config.AuditConfig{File: path}, zap.NewNop())
	require.NotNil(t, n)
	assert.Len(t, n.observers, 1)
}

func TestNewNotifier_URLOnly(t *testing.T) {
	n := NewNotifier(&config.AuditConfig{URL: "http://example.invalid/audit"}, zap.NewNop())
	require.NotNil(t, n)
	assert.Len(t, n.observers, 1)
}

func TestNewNotifier_FileAndURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	n := NewNotifier(&config.AuditConfig{File: path, URL: "http://example.invalid/audit"}, zap.NewNop())
	require.NotNil(t, n)
	assert.Len(t, n.observers, 2)
}

func TestNotifier_Notify_CallsAllObservers(t *testing.T) {
	obs1 := &fakeObserver{name: "obs1"}
	obs2 := &fakeObserver{name: "obs2"}
	n := &Notifier{
		observers: []Observer{obs1, obs2},
		logger:    zap.NewNop(),
	}

	event := NewEvent([]string{"cpu"}, "1.1.1.1")
	n.Notify(event)
	require.NoError(t, n.Shutdown(context.Background()))

	assert.Equal(t, 1, obs1.calls)
	assert.Equal(t, 1, obs2.calls)
	assert.Equal(t, []Event{event}, obs1.received)
	assert.Equal(t, []Event{event}, obs2.received)
}

func TestNotifier_Notify_ContinuesAfterObserverError(t *testing.T) {
	failing := &fakeObserver{name: "failing", err: assert.AnError}
	ok := &fakeObserver{name: "ok"}
	n := &Notifier{
		observers: []Observer{failing, ok},
		logger:    zap.NewNop(),
	}

	event := NewEvent([]string{"cpu"}, "1.1.1.1")
	n.Notify(event)
	require.NoError(t, n.Shutdown(context.Background()))

	assert.Equal(t, 1, failing.calls)
	assert.Equal(t, 1, ok.calls)
}

func TestNotifier_Notify_IsNonBlocking(t *testing.T) {
	obs1 := &fakeSlowCloserObserver{name: "obs1", delay: 100 * time.Millisecond}
	obs2 := &fakeSlowCloserObserver{name: "obs2", delay: 100 * time.Millisecond}
	n := &Notifier{
		observers: []Observer{obs1, obs2},
		logger:    zap.NewNop(),
	}

	start := time.Now()
	n.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 150*time.Millisecond)

	require.NoError(t, n.Shutdown(context.Background()))
	assert.Equal(t, int32(1), obs1.calls.Load())
	assert.Equal(t, int32(1), obs2.calls.Load())
}

func TestNotifier_Notify_NilReceiver_DoesNotPanic(t *testing.T) {
	var n *Notifier
	assert.NotPanics(t, func() {
		n.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))
	})
}

func TestNotifier_Shutdown_WaitsForInFlightNotify(t *testing.T) {
	obs := &fakeSlowCloserObserver{name: "obs", delay: 50 * time.Millisecond}
	n := &Notifier{
		observers: []Observer{obs},
		logger:    zap.NewNop(),
	}

	n.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))
	require.NoError(t, n.Shutdown(context.Background()))

	assert.Equal(t, int32(1), obs.calls.Load())
}

func TestNotifier_Shutdown_TimesOutAndSkipsClose(t *testing.T) {
	obs := &fakeSlowCloserObserver{name: "obs", delay: 200 * time.Millisecond}
	n := &Notifier{
		observers: []Observer{obs},
		logger:    zap.NewNop(),
	}

	n.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := n.Shutdown(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, obs.closeCalled.Load())
}

func TestNotifier_Shutdown_ClosesAllObservers(t *testing.T) {
	obs1 := &fakeSlowCloserObserver{name: "obs1"}
	obs2 := &fakeSlowCloserObserver{name: "obs2"}
	n := &Notifier{
		observers: []Observer{obs1, obs2},
		logger:    zap.NewNop(),
	}

	n.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))
	require.NoError(t, n.Shutdown(context.Background()))

	assert.True(t, obs1.closeCalled.Load())
	assert.True(t, obs2.closeCalled.Load())
}

func TestNotifier_Shutdown_ContinuesClosingAfterCloseError(t *testing.T) {
	failing := &fakeSlowCloserObserver{name: "failing", closeErr: assert.AnError}
	ok := &fakeSlowCloserObserver{name: "ok"}
	n := &Notifier{
		observers: []Observer{failing, ok},
		logger:    zap.NewNop(),
	}

	n.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))
	err := n.Shutdown(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	assert.True(t, failing.closeCalled.Load())
	assert.True(t, ok.closeCalled.Load())
}

func TestNotifier_Shutdown_SkipsObserversWithoutClose(t *testing.T) {
	nonCloser := &fakeObserver{name: "non-closer"}
	n := &Notifier{
		observers: []Observer{nonCloser},
		logger:    zap.NewNop(),
	}

	n.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))

	var err error
	assert.NotPanics(t, func() {
		err = n.Shutdown(context.Background())
	})
	assert.NoError(t, err)
}
