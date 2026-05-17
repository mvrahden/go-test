package notification

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
)

type Notification struct {
	To       string   `json:"to"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	Priority Priority `json:"priority"`
}

type delivery struct {
	Notification
	DeliveredAt time.Time `json:"delivered_at"`
}

type dispatcher struct {
	mu    sync.Mutex
	log   []delivery
	count atomic.Int32
}

func newDispatcher() *dispatcher {
	return &dispatcher{}
}

func (d *dispatcher) Send(n Notification) {
	go func() {
		time.Sleep(50 * time.Millisecond)
		d.mu.Lock()
		defer d.mu.Unlock()
		d.log = append(d.log, delivery{Notification: n, DeliveredAt: time.Now()})
		d.count.Add(1)
	}()
}

func (d *dispatcher) Deliveries() []delivery {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]delivery, len(d.log))
	copy(out, d.log)
	return out
}

func (d *dispatcher) DeliveryCount() int {
	return int(d.count.Load())
}

func formatSummary(d delivery) string {
	return fmt.Sprintf("[%s] %s → %s", priorityLabel(d.Priority), d.Subject, d.To)
}

func priorityLabel(p Priority) string {
	switch p {
	case PriorityHigh:
		return "HIGH"
	case PriorityNormal:
		return "NORMAL"
	default:
		return "LOW"
	}
}
