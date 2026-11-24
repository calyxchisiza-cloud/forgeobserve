package forgeobserve

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type writer interface {
	metricName() string
	writeProm(b *strings.Builder)
}

// Registry holds the metrics for one process.
type Registry struct {
	mu    sync.Mutex
	names map[string]string
	items []writer
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{names: map[string]string{}}
}

// Counter registers a counter. The name cannot be registered again.
func (r *Registry) Counter(name, help string) (*Counter, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	if err := validHelp(help); err != nil {
		return nil, err
	}
	c := &Counter{name: name, help: help, value: &atomic.Uint64{}}
	if err := r.add(name, "counter", c); err != nil {
		return nil, err
	}
	return c, nil
}

// Gauge registers a gauge.
func (r *Registry) Gauge(name, help string) (*Gauge, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	if err := validHelp(help); err != nil {
		return nil, err
	}
	g := &Gauge{name: name, help: help, bits: &atomic.Uint64{}}
	if err := r.add(name, "gauge", g); err != nil {
		return nil, err
	}
	return g, nil
}

// Histogram registers a histogram. Bounds are finite, strictly increasing, and non-negative.
func (r *Registry) Histogram(name, help string, bounds []float64) (*Histogram, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	if err := validHelp(help); err != nil {
		return nil, err
	}
	if len(bounds) == 0 {
		return nil, fmt.Errorf("%w: bounds are required", ErrInvalid)
	}
	clean := make([]float64, len(bounds))
	for i, bound := range bounds {
		if math.IsNaN(bound) || math.IsInf(bound, 0) || bound < 0 {
			return nil, fmt.Errorf("%w: bound is not valid", ErrInvalid)
		}
		if i > 0 && bound <= bounds[i-1] {
			return nil, fmt.Errorf("%w: bounds are not increasing", ErrInvalid)
		}
		clean[i] = bound
	}
	h := &Histogram{
		name:   name,
		help:   help,
		bounds: clean,
		base:   newHistSeries("", clean),
	}
	if err := r.add(name, "histogram", h); err != nil {
		return nil, err
	}
	return h, nil
}

func (r *Registry) add(name, kind string, item writer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if have, ok := r.names[name]; ok {
		return fmt.Errorf("%w: %s is already a %s", ErrInvalid, name, have)
	}
	r.names[name] = kind
	r.items = append(r.items, item)
	return nil
}

func (c *Counter) metricName() string   { return c.name }
func (g *Gauge) metricName() string     { return g.name }
func (h *Histogram) metricName() string { return h.name }

// Text renders the registry in the Prometheus text format.
func (r *Registry) Text() string {
	r.mu.Lock()
	items := append([]writer(nil), r.items...)
	r.mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].metricName() < items[j].metricName() })
	var b strings.Builder
	for _, item := range items {
		item.writeProm(&b)
	}
	return b.String()
}
