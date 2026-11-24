package forgeobserve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type spanKey struct{}

// TraceConfig controls the span ring and the clock. A zero Limit keeps 128 spans.
// NewID, when set, supplies the next id and must be safe for concurrent calls.
type TraceConfig struct {
	Limit int
	Now   func() time.Time
	NewID func() string
}

// Finished is one completed span.
type Finished struct {
	TraceID  string
	SpanID   string
	ParentID string
	Name     string
	Start    time.Time
	End      time.Time
	Err      string
	Attrs    map[string]string
}

// Status is ok, or error when the span recorded one.
func (f Finished) Status() string {
	if f.Err == "" {
		return "ok"
	}
	return "error"
}

// Duration is the time from start to end.
func (f Finished) Duration() time.Duration { return f.End.Sub(f.Start) }

// Tracer keeps completed spans for this process.
type Tracer struct {
	limit int
	now   func() time.Time
	newID func() string
	mu    sync.Mutex
	done  []Finished
}

// Span is an open unit of work. End records it.
type Span struct {
	tracer   *Tracer
	traceID  string
	spanID   string
	parentID string
	name     string
	start    time.Time
	err      string
	attrs    map[string]string
	ended    bool
	mu       sync.Mutex
}

// NewTracer builds a tracer. A negative limit is rejected.
func NewTracer(cfg TraceConfig) (*Tracer, error) {
	if cfg.Limit < 0 {
		return nil, fmt.Errorf("%w: limit is not valid", ErrInvalid)
	}
	limit := cfg.Limit
	if limit == 0 {
		limit = 128
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	newID := cfg.NewID
	if newID == nil {
		newID = randomID
	}
	return &Tracer{limit: limit, now: now, newID: newID}, nil
}

// Start opens a span. A span already in ctx is the parent, and it shares the trace id.
func (t *Tracer) Start(ctx context.Context, name string) (context.Context, *Span, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !validSpanName(name) {
		return ctx, nil, fmt.Errorf("%w: span name is not valid", ErrInvalid)
	}
	sp := &Span{
		tracer: t,
		name:   name,
		start:  t.now(),
	}
	if parent := SpanFrom(ctx); parent != nil {
		sp.traceID = parent.traceID
		sp.parentID = parent.spanID
		sp.spanID = t.newID()
	} else {
		sp.traceID = t.newID()
		sp.spanID = t.newID()
	}
	return context.WithValue(ctx, spanKey{}, sp), sp, nil
}

func validSpanName(name string) bool {
	if name == "" || len(name) > maxName {
		return false
	}
	for _, r := range name {
		if r <= ' ' || r == '{' || r == '}' {
			return false
		}
	}
	return true
}

// SpanFrom returns the span stored in ctx.
func SpanFrom(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	sp, _ := ctx.Value(spanKey{}).(*Span)
	return sp
}

// Name is the span name.
func (s *Span) Name() string { return s.name }

// TraceID is shared by a span and its children.
func (s *Span) TraceID() string { return s.traceID }

// SpanID identifies this span.
func (s *Span) SpanID() string { return s.spanID }

// ParentID is empty for a root span.
func (s *Span) ParentID() string { return s.parentID }

// Attr records one attribute. A call after End is ignored.
func (s *Span) Attr(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended || key == "" || len(key) > maxLabel || len(value) > maxValue {
		return
	}
	if s.attrs == nil {
		s.attrs = map[string]string{}
	}
	s.attrs[key] = value
}

// SetError marks the span failed. A call after End is ignored.
func (s *Span) SetError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.err = clip(err.Error(), maxMessage)
}

// End records the span. A second call does nothing.
func (s *Span) End() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	done := Finished{
		TraceID:  s.traceID,
		SpanID:   s.spanID,
		ParentID: s.parentID,
		Name:     s.name,
		Start:    s.start,
		End:      s.tracer.now(),
		Err:      s.err,
		Attrs:    copyAttrs(s.attrs),
	}
	s.mu.Unlock()
	s.tracer.record(done)
}

// Spans returns completed spans, oldest retained first.
func (t *Tracer) Spans() []Finished {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Finished, len(t.done))
	for i, sp := range t.done {
		sp.Attrs = copyAttrs(sp.Attrs)
		out[i] = sp
	}
	return out
}

func (t *Tracer) record(sp Finished) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done = append(t.done, sp)
	if len(t.done) > t.limit {
		next := make([]Finished, t.limit)
		copy(next, t.done[len(t.done)-t.limit:])
		t.done = next
	}
}

func copyAttrs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func randomID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
