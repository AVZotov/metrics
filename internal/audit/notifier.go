package audit

import (
	"go.uber.org/zap"
)

type Notifier struct {
	observers []Observer
	logger    *zap.Logger
}

//func NewNotifier(cfg *config.ServerConfig, logger *zap.Logger) *Notifier {
//	observers := make([]Observer, 2)
//
//	return &Notifier{
//		observers: make([]Observer, 0),
//		logger:    logger,
//	}
//}

func (n *Notifier) register(o Observer) {
	n.observers = append(n.observers, o)
}

func (n *Notifier) Notify(e Event) {
	for _, observer := range n.observers {
		if err := observer.Notify(e); err != nil {
			n.logger.Warn("audit observer failed", zap.String("observer ", observer.Name()), zap.Error(err))
		}
	}
}
