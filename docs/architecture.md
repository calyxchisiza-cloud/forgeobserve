# Architecture

ForgeObserve is in-process. It does not ship samples to another machine. A scraper reads `/metrics`. The text is the Prometheus exposition format: a help line, a type line, then one sample per series.

Counters are unsigned and atomic. Gauges store a float the same way, and `Add` retries if another update lands first. A histogram locks its own series so the count, the sum, and the cumulative buckets move together. A sample equal to a bound is counted in that bound. `+Inf` counts every sample.

Label sets are sorted by key. The rendered `{code="500"}` is the map key, so the same set returns the same series. The unlabeled series stays separate.

A tracer assigns a trace id to a root span and a new span id to every span. A child copies the trace id and stores the parent's span id. `End` copies the name, the clock, the error, and the attributes into the ring. The ring is a fixed slice. Past the limit, the oldest finished span is dropped. Spans that are still open are not in the ring.

The log is the same kind of ring. Levels are `info` and `error`. A message is one line.

`/health` is only a process answer. It does not check a backend. v1 has one registry, one tracer, and one log in the process that created them.
