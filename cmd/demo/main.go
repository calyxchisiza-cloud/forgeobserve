package main

import (
	"context"
	"fmt"
	"os"

	"forgeobserve"
)

func main() {
	reg := forgeobserve.New()
	requests, err := reg.Counter("requests", "Handled requests.")
	if err != nil {
		fail(err)
	}
	errs, err := reg.Counter("errors", "Failed requests.")
	if err != nil {
		fail(err)
	}
	latency, err := reg.Histogram("latency_ms", "Latency in milliseconds.", []float64{10, 100})
	if err != nil {
		fail(err)
	}
	requests.Add(2)
	errs.Add(1)
	if err := latency.Observe(5); err != nil {
		fail(err)
	}
	if err := latency.Observe(15); err != nil {
		fail(err)
	}
	tr, err := forgeobserve.NewTracer(forgeobserve.TraceConfig{})
	if err != nil {
		fail(err)
	}
	_, span, err := tr.Start(context.Background(), "demo")
	if err != nil {
		fail(err)
	}
	span.End()
	fmt.Printf("requests %d\n", requests.Value())
	fmt.Printf("errors %d\n", errs.Value())
	fmt.Printf("latency_ms_count %d\n", latency.Count())
	fmt.Printf("latency_ms_sum %g\n", latency.Sum())
	fmt.Printf("%s ok\n", span.Name())
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
