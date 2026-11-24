# ForgeObserve

ForgeObserve keeps the numbers and traces for one process. A counter only increases. A gauge can rise or fall. A histogram counts samples into buckets. Finished spans stay in a ring, and a short log keeps the latest diagnostic lines.

`Text` renders the registry in the Prometheus text format. `Handler` serves that scrape, plus the recent spans, the log, and a health line.

```go
reg := forgeobserve.New()
requests, err := reg.Counter("requests", "Handled requests.")
requests.Add(1)
fmt.Print(reg.Text())

tr, err := forgeobserve.NewTracer(forgeobserve.TraceConfig{})
ctx, span, err := tr.Start(context.Background(), "demo")
span.End()
```

A labeled series does not change the unlabeled total. `With("code", "500")` is its own counter. Thirty-two label sets fit on one metric. The next one is rejected.

A span opened from another span's context keeps the trace id and records the parent. `End` stores the span. A second `End` does nothing. The ring drops the oldest span after the limit. The default limit is 128.

## Run

```
go run ./cmd/demo
```

The demo prints:

```
requests 2
errors 1
latency_ms_count 2
latency_ms_sum 20
demo ok
```

```
go run ./cmd/forgeobserve -listen 127.0.0.1:8095
```

`GET /metrics` is the scrape. `GET /traces` lists finished spans. `GET /events` lists the log. `GET /health` returns `ok`. The listener stays on loopback until interrupt.

## Test

```
go test ./...
go test -race -count=1 ./...
```
