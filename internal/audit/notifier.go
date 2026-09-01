package audit

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/AVZotov/metrics/internal/config"
	"go.uber.org/zap"
)

// Notifier fans out audit events to a set of registered observers.
type Notifier struct {
	observers []Observer
	logger    *zap.Logger
	wg        sync.WaitGroup
}

// NewNotifier builds a Notifier and registers a file and/or HTTP observer
// based on cfg. A failed file observer setup is logged and skipped, not fatal.
func NewNotifier(cfg *config.AuditConfig, logger *zap.Logger) *Notifier {
	observers := make([]Observer, 0)
	n := Notifier{
		observers: observers,
		logger:    logger,
	}
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

	return &n
}

func (n *Notifier) register(o Observer) {
	n.observers = append(n.observers, o)
}

// Notify sends the event to all registered observers concurrently. Safe to
// call on a nil Notifier (no-op). Observer errors are logged, not returned.
func (n *Notifier) Notify(e Event) {
	if n == nil {
		return
	}

	for _, observer := range n.observers {
		n.wg.Add(1)
		go func(o Observer) {
			defer n.wg.Done()
			if err := o.Notify(e); err != nil {
				n.logger.Warn("audit observer failed", zap.String("observer", o.Name()), zap.Error(err))
			}
		}(observer)
	}
}

// Shutdown waits for in-flight notifications to finish and closes any
// observers that implement io.Closer. Returns ctx.Err() if ctx is done
// before pending notifications complete, or a joined error if closing any observer fails.
func (n *Notifier) Shutdown(ctx context.Context) error {
	if n == nil {
		return nil
	}
	var shutdownErr error
	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		n.logger.Warn("audit shutdown timed out, some events may be lost")
		return ctx.Err()
	}

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
