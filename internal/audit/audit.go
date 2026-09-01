package audit

import "time"

// Event is a single audit record: which metrics were touched, when, and by whom.
type Event struct {
	Timestamp int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IPAddress string   `json:"ip_address"`
}

// NewEvent creates an Event with the current timestamp for the given metrics and client address.
func NewEvent(metrics []string, addr string) Event {
	return Event{Timestamp: time.Now().Unix(), Metrics: metrics, IPAddress: addr}
}

// Observer receives audit events. Notify delivers an event and returns an
// error if the observer couldn't record it. Name identifies the observer for logging.
type Observer interface {
	Notify(event Event) error
	Name() string
}
