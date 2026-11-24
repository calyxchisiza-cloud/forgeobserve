package forgeobserve

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Counter is a value that only increases.
type Counter struct {
	root  *Counter
	name  string
	help  string
	value *atomic.Uint64
	mu    sync.Mutex
	kids  map[string]*atomic.Uint64
	order []string
}

// Add increases the counter by n.
func (c *Counter) Add(n uint64) { c.value.Add(n) }

// Value is this series.
func (c *Counter) Value() uint64 { return c.value.Load() }

// With returns the counter for one label set. The parent total does not change.
func (c *Counter) With(kv ...string) (*Counter, error) {
	labels, err := labelsOf(kv)
	if err != nil {
		return nil, err
	}
	root := c.family()
	root.mu.Lock()
	defer root.mu.Unlock()
	if kid := root.kids[labels]; kid != nil {
		return &Counter{root: root, name: root.name, help: root.help, value: kid}, nil
	}
	if len(root.kids) >= maxSeries {
		return nil, fmt.Errorf("%w: too many label sets", ErrInvalid)
	}
	kid := &atomic.Uint64{}
	if root.kids == nil {
		root.kids = map[string]*atomic.Uint64{}
	}
	root.kids[labels] = kid
	root.order = append(root.order, labels)
	return &Counter{root: root, name: root.name, help: root.help, value: kid}, nil
}

func (c *Counter) family() *Counter {
	if c.root != nil {
		return c.root
	}
	return c
}

func (c *Counter) writeProm(b *strings.Builder) {
	c = c.family()
	writeHead(b, c.name, c.help, "counter")
	fmt.Fprintf(b, "%s %d\n", c.name, c.value.Load())
	c.mu.Lock()
	order := append([]string(nil), c.order...)
	kids := make(map[string]*atomic.Uint64, len(c.kids))
	for k, v := range c.kids {
		kids[k] = v
	}
	c.mu.Unlock()
	sortLabels(order)
	for _, labels := range order {
		fmt.Fprintf(b, "%s%s %d\n", c.name, labels, kids[labels].Load())
	}
}

// Gauge is a value that can rise or fall.
type Gauge struct {
	root  *Gauge
	name  string
	help  string
	bits  *atomic.Uint64
	mu    sync.Mutex
	kids  map[string]*atomic.Uint64
	order []string
}

// Set stores v. NaN and infinities are rejected.
func (g *Gauge) Set(v float64) error {
	if err := finite(v); err != nil {
		return err
	}
	g.bits.Store(math.Float64bits(v))
	return nil
}

// Add changes the gauge by v.
func (g *Gauge) Add(v float64) error {
	if err := finite(v); err != nil {
		return err
	}
	for {
		old := g.bits.Load()
		next := math.Float64frombits(old) + v
		if err := finite(next); err != nil {
			return err
		}
		if g.bits.CompareAndSwap(old, math.Float64bits(next)) {
			return nil
		}
	}
}

// Value is this series.
func (g *Gauge) Value() float64 { return math.Float64frombits(g.bits.Load()) }

// With returns the gauge for one label set.
func (g *Gauge) With(kv ...string) (*Gauge, error) {
	labels, err := labelsOf(kv)
	if err != nil {
		return nil, err
	}
	root := g.family()
	root.mu.Lock()
	defer root.mu.Unlock()
	if kid := root.kids[labels]; kid != nil {
		return &Gauge{root: root, name: root.name, help: root.help, bits: kid}, nil
	}
	if len(root.kids) >= maxSeries {
		return nil, fmt.Errorf("%w: too many label sets", ErrInvalid)
	}
	kid := &atomic.Uint64{}
	if root.kids == nil {
		root.kids = map[string]*atomic.Uint64{}
	}
	root.kids[labels] = kid
	root.order = append(root.order, labels)
	return &Gauge{root: root, name: root.name, help: root.help, bits: kid}, nil
}

func (g *Gauge) family() *Gauge {
	if g.root != nil {
		return g.root
	}
	return g
}

func (g *Gauge) writeProm(b *strings.Builder) {
	g = g.family()
	writeHead(b, g.name, g.help, "gauge")
	fmt.Fprintf(b, "%s %s\n", g.name, formatFloat(g.Value()))
	g.mu.Lock()
	order := append([]string(nil), g.order...)
	kids := make(map[string]*atomic.Uint64, len(g.kids))
	for k, v := range g.kids {
		kids[k] = v
	}
	g.mu.Unlock()
	sortLabels(order)
	for _, labels := range order {
		v := math.Float64frombits(kids[labels].Load())
		fmt.Fprintf(b, "%s%s %s\n", g.name, labels, formatFloat(v))
	}
}

// Histogram counts observations into increasing buckets.
type Histogram struct {
	root   *Histogram
	name   string
	help   string
	bounds []float64
	base   *histSeries
	mu     sync.Mutex
	kids   map[string]*histSeries
	order  []string
}

type histSeries struct {
	labels  string
	bounds  []float64
	count   uint64
	sum     float64
	buckets []uint64
	mu      sync.Mutex
}

// Observe records one non-negative sample. A value equal to a bound is included.
func (h *Histogram) Observe(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return fmt.Errorf("%w: sample is not valid", ErrInvalid)
	}
	h.base.observe(v)
	return nil
}

// Count is the number of unlabeled samples.
func (h *Histogram) Count() uint64 {
	h.base.mu.Lock()
	defer h.base.mu.Unlock()
	return h.base.count
}

// Sum is the total of unlabeled samples.
func (h *Histogram) Sum() float64 {
	h.base.mu.Lock()
	defer h.base.mu.Unlock()
	return h.base.sum
}

// With returns the histogram for one label set.
func (h *Histogram) With(kv ...string) (*Histogram, error) {
	labels, err := labelsOf(kv)
	if err != nil {
		return nil, err
	}
	root := h.family()
	root.mu.Lock()
	defer root.mu.Unlock()
	if kid := root.kids[labels]; kid != nil {
		return &Histogram{root: root, name: root.name, help: root.help, bounds: root.bounds, base: kid}, nil
	}
	if len(root.kids) >= maxSeries {
		return nil, fmt.Errorf("%w: too many label sets", ErrInvalid)
	}
	kid := newHistSeries(labels, root.bounds)
	if root.kids == nil {
		root.kids = map[string]*histSeries{}
	}
	root.kids[labels] = kid
	root.order = append(root.order, labels)
	return &Histogram{root: root, name: root.name, help: root.help, bounds: root.bounds, base: kid}, nil
}

func (h *Histogram) family() *Histogram {
	if h.root != nil {
		return h.root
	}
	return h
}

func newHistSeries(labels string, bounds []float64) *histSeries {
	return &histSeries{
		labels:  labels,
		bounds:  bounds,
		buckets: make([]uint64, len(bounds)+1),
	}
}

func (s *histSeries) observe(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	s.sum += v
	for i, bound := range s.bounds {
		if v <= bound {
			s.buckets[i]++
		}
	}
	s.buckets[len(s.buckets)-1]++
}

func (h *Histogram) writeProm(b *strings.Builder) {
	h = h.family()
	writeHead(b, h.name, h.help, "histogram")
	h.mu.Lock()
	order := append([]string(nil), h.order...)
	kids := make([]*histSeries, 0, 1+len(h.kids))
	kids = append(kids, h.base)
	sortLabels(order)
	for _, labels := range order {
		kids = append(kids, h.kids[labels])
	}
	h.mu.Unlock()
	for _, kid := range kids {
		kid.write(b, h.name)
	}
}

func (s *histSeries) write(b *strings.Builder, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, bound := range s.bounds {
		fmt.Fprintf(b, "%s %d\n", bucketLine(name, s.labels, formatFloat(bound)), s.buckets[i])
	}
	fmt.Fprintf(b, "%s %d\n", bucketLine(name, s.labels, "+Inf"), s.buckets[len(s.buckets)-1])
	fmt.Fprintf(b, "%s_sum%s %s\n", name, s.labels, formatFloat(s.sum))
	fmt.Fprintf(b, "%s_count%s %d\n", name, s.labels, s.count)
}

func bucketLine(name, labels, le string) string {
	if labels == "" {
		return fmt.Sprintf("%s_bucket{le=\"%s\"}", name, le)
	}
	return fmt.Sprintf("%s_bucket{%s,le=\"%s\"}", name, labels[1:len(labels)-1], le)
}

func finite(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%w: value is not finite", ErrInvalid)
	}
	return nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func writeHead(b *strings.Builder, name, help, kind string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, escape(help, false))
	fmt.Fprintf(b, "# TYPE %s %s\n", name, kind)
}

func sortLabels(order []string) {
	sort.Strings(order)
}
