package audit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewEvent(t *testing.T) {
	before := time.Now().Unix()
	metrics := []string{"cpu", "hits"}
	ip := "127.0.0.1"

	event := NewEvent(metrics, ip)

	after := time.Now().Unix()

	assert.GreaterOrEqual(t, event.Timestamp, before)
	assert.LessOrEqual(t, event.Timestamp, after)
	assert.Equal(t, metrics, event.Metrics)
	assert.Equal(t, ip, event.IPAddress)
}
