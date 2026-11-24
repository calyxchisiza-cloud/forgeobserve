// Package forgeobserve keeps process metrics, traces, and a short diagnostic log.
// Counters, gauges, and histograms stay in memory. A finished span is kept in
// a ring. Handler serves /metrics, /traces, /events, and /health for a scrape.
package forgeobserve
