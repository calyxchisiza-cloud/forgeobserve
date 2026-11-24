package forgeobserve

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Event is one diagnostic line.
type Event struct {
	Time    time.Time
	Level   string
	Message string
}

// Log is a ring of recent diagnostic events.
type Log struct {
	limit int
	now   func() time.Time
	mu    sync.Mutex
	items []Event
}

// NewLog keeps the most recent events. A zero limit keeps 128. A negative limit is rejected.
func NewLog(limit int, now func() time.Time) (*Log, error) {
	if limit < 0 {
		return nil, fmt.Errorf("%w: limit is not valid", ErrInvalid)
	}
	if limit == 0 {
		limit = 128
	}
	if now == nil {
		now = time.Now
	}
	return &Log{limit: limit, now: now}, nil
}

// Write appends an info or error event. The message must be one line.
func (l *Log) Write(level, message string) error {
	if level != "info" && level != "error" {
		return fmt.Errorf("%w: level is not valid", ErrInvalid)
	}
	if message == "" || len(message) > maxMessage || strings.ContainsAny(message, "\r\n") {
		return fmt.Errorf("%w: message is not valid", ErrInvalid)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, Event{Time: l.now(), Level: level, Message: message})
	if len(l.items) > l.limit {
		next := make([]Event, l.limit)
		copy(next, l.items[len(l.items)-l.limit:])
		l.items = next
	}
	return nil
}

// Events returns the retained lines, oldest first.
func (l *Log) Events() []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Event, len(l.items))
	copy(out, l.items)
	return out
}
