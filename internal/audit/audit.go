package audit

import "time"

type Event struct {
	Timestamp int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IPAddress string   `json:"ip_address"`
}

func NewEvent(metrics []string, addr string) Event {
	return Event{Timestamp: time.Now().Unix(), Metrics: metrics, IPAddress: addr}
}

type Observer interface {
	Notify(event Event) error
	Name() string
}
