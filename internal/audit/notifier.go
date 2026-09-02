package audit

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/AVZotov/metrics/internal/config"
	"go.uber.org/zap"
)

// eventBufferSize bounds how many audit events can queue up waiting for the
// worker to dispatch them. Sized well above a typical request burst so
// Notify's non-blocking send only has to drop events if the worker is
// stalled (e.g. a slow HTTP observer) for an unusually long time, while
// still capping memory if that happens.
const eventBufferSize = 100

// Notifier fans out audit events to a set of registered observers. Events
// are queued on a channel and delivered by a single background worker, so
// Notify never blocks the caller and never races with Shutdown.
type Notifier struct {
	observers []Observer
	logger    *zap.Logger
	events    chan Event
	done      chan struct{}

	mu     sync.RWMutex
	closed bool
}

// NewNotifier builds a Notifier and registers a file and/or HTTP observer
// based on cfg. A failed file observer setup is logged and skipped, not fatal.
func NewNotifier(cfg *config.AuditConfig, logger *zap.Logger) *Notifier {
	n := newNotifier(nil, logger)
	if cfg.File != "" {
		fileObserver, err := newFileObserver(cfg.File)
		if err != nil {
			n.logger.Warn("audit observer failed", zap.String("file observer", err.Error()))
		} else {
			n.register(fileObserver)
		}
	}
	if cfg.URL != "" {
		httpObserver := newHTTPObserver(cfg.URL)
		n.register(httpObserver)
	}

	return n
}

// newNotifier builds a Notifier with the given observers and starts its
// background dispatch worker. Split out from NewNotifier so tests can wire
// up fake observers directly without going through config-driven setup.
func newNotifier(observers []Observer, logger *zap.Logger) *Notifier {
	n := &Notifier{
		observers: observers,
		logger:    logger,
		events:    make(chan Event, eventBufferSize),
		done:      make(chan struct{}),
	}
	go n.worker()
	return n
}

func (n *Notifier) register(o Observer) {
	n.observers = append(n.observers, o)
}

// worker drains events until the channel is closed by Shutdown, dispatching
// each to every observer in turn before signaling completion via done.
func (n *Notifier) worker() {
	defer close(n.done)
	for e := range n.events {
		n.dispatch(e)
	}
}

func (n *Notifier) dispatch(e Event) {
	for _, observer := range n.observers {
		if err := observer.Notify(e); err != nil {
			n.logger.Warn("audit observer failed", zap.String("observer", observer.Name()), zap.Error(err))
		}
	}
}

// Notify queues the event for delivery to all registered observers. Safe to
// call on a nil Notifier (no-op) and safe to call concurrently with or after
// Shutdown (the event is silently dropped once shutdown has begun). Never
// blocks the caller: if the queue is full, the event is dropped and a
// warning is logged rather than stalling the request path.
func (n *Notifier) Notify(e Event) {
	if n == nil {
		return
	}

	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.closed {
		return
	}

	select {
	case n.events <- e:
	default:
		n.logger.Warn("audit event dropped, queue full")
	}
}

// Shutdown stops accepting new events, waits for the worker to drain
// in-flight events, and closes any observers that implement io.Closer.
// Returns ctx.Err() if ctx is done before draining completes, or a joined
// error if closing any observer fails.
func (n *Notifier) Shutdown(ctx context.Context) error {
	if n == nil {
		return nil
	}

	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		<-n.done
		return nil
	}
	n.closed = true
	close(n.events)
	n.mu.Unlock()

	select {
	case <-n.done:
	case <-ctx.Done():
		n.logger.Warn("audit shutdown timed out, some events may be lost")
		return ctx.Err()
	}

	var shutdownErr error
	for _, o := range n.observers {
		if closer, ok := o.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				n.logger.Warn("failed to close audit observer", zap.String("observer", o.Name()), zap.Error(err))
				shutdownErr = errors.Join(shutdownErr, err)
			}
		}
	}
	return shutdownErr
}
