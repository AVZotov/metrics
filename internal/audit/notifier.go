package audit

import (
	"github.com/AVZotov/metrics/internal/config"
	"go.uber.org/zap"
)

type Notifier struct {
	observers []Observer
	logger    *zap.Logger
}

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

func (n *Notifier) Notify(e Event) {
	for _, observer := range n.observers {
		if err := observer.Notify(e); err != nil {
			n.logger.Warn("audit observer failed", zap.String("observer", observer.Name()), zap.Error(err))
		}
	}
}
