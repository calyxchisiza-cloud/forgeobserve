package forgeobserve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetricsText(t *testing.T) {
	reg := New()
	requests, err := reg.Counter("requests", "Handled requests.")
	if err != nil {
		t.Fatal(err)
	}
	requests.Add(2)
	failed, err := requests.With("code", `5"00`)
	if err != nil {
		t.Fatal(err)
	}
	failed.Add(1)
	if requests.Value() != 2 || failed.Value() != 1 {
		t.Fatalf("requests %d failed %d", requests.Value(), failed.Value())
	}
	latency, err := reg.Histogram("latency_ms", "Latency in milliseconds.", []float64{10, 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := latency.Observe(5); err != nil {
		t.Fatal(err)
	}
	if err := latency.Observe(15); err != nil {
		t.Fatal(err)
	}
	errs, err := reg.Counter("errors", "Failed requests.")
	if err != nil {
		t.Fatal(err)
	}
	errs.Add(1)
	got := reg.Text()
	want := "" +
		"# HELP errors Failed requests.\n" +
		"# TYPE errors counter\n" +
		"errors 1\n" +
		"# HELP latency_ms Latency in milliseconds.\n" +
		"# TYPE latency_ms histogram\n" +
		"latency_ms_bucket{le=\"10\"} 1\n" +
		"latency_ms_bucket{le=\"100\"} 2\n" +
		"latency_ms_bucket{le=\"+Inf\"} 2\n" +
		"latency_ms_sum 20\n" +
		"latency_ms_count 2\n" +
		"# HELP requests Handled requests.\n" +
		"# TYPE requests counter\n" +
		"requests 2\n" +
		"requests{code=\"5\\\"00\"} 1\n"
	if got != want {
		t.Fatalf("text\n%s", got)
	}
	if latency.Count() != 2 || latency.Sum() != 20 {
		t.Fatalf("count %d sum %v", latency.Count(), latency.Sum())
	}
}

func TestGaugeMoves(t *testing.T) {
	reg := New()
	g, err := reg.Gauge("inflight", "In flight.")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Set(3); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(-1); err != nil {
		t.Fatal(err)
	}
	if g.Value() != 2 {
		t.Fatalf("value %v", g.Value())
	}
	if err := g.Set(math.NaN()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nan: %v", err)
	}
	route, err := g.With("path", "/api")
	if err != nil {
		t.Fatal(err)
	}
	if err := route.Set(4); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reg.Text(), "inflight{path=\"/api\"} 4\n") {
		t.Fatalf("text\n%s", reg.Text())
	}
}

func TestRejectedMetrics(t *testing.T) {
	reg := New()
	if _, err := reg.Counter("bad name", "Help."); !errors.Is(err, ErrInvalid) {
		t.Fatalf("name: %v", err)
	}
	if _, err := reg.Counter("requests", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("help: %v", err)
	}
	if _, err := reg.Counter("requests", "Handled requests."); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Gauge("requests", "Other."); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := reg.Histogram("latency_ms", "Latency.", []float64{10, 5}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bounds: %v", err)
	}
	h, err := reg.Histogram("latency_ms", "Latency.", []float64{10})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Observe(-1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("sample: %v", err)
	}
	c, err := reg.Counter("codes", "Codes.")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxSeries; i++ {
		if _, err := c.With("n", fmt.Sprintf("%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.With("n", "more"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cardinality: %v", err)
	}
}

func TestSpansKeepTheTraceAndDropTheOldest(t *testing.T) {
	var n int
	tr, err := NewTracer(TraceConfig{
		Limit: 2,
		Now:   func() time.Time { return time.Unix(10, 0).UTC() },
		NewID: func() string {
			n++
			return fmt.Sprintf("id%02d", n)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, root, err := tr.Start(context.Background(), "root")
	if err != nil {
		t.Fatal(err)
	}
	ctx, child, err := tr.Start(ctx, "child")
	if err != nil {
		t.Fatal(err)
	}
	if child.TraceID() != root.TraceID() || child.ParentID() != root.SpanID() || root.ParentID() != "" {
		t.Fatalf("root %s %s child %s parent %s", root.TraceID(), root.SpanID(), child.TraceID(), child.ParentID())
	}
	child.Attr("k", "v")
	child.SetError(errors.New("nope"))
	child.End()
	child.End()
	child.Attr("late", "x")
	root.End()
	_, extra, err := tr.Start(context.Background(), "extra")
	if err != nil {
		t.Fatal(err)
	}
	extra.End()
	got := tr.Spans()
	if len(got) != 2 || got[0].Name != "root" || got[1].Name != "extra" {
		t.Fatalf("%+v", got)
	}
	if got[0].TraceID != "id01" || got[0].SpanID != "id02" || got[0].Status() != "ok" || got[0].Duration() != 0 {
		t.Fatalf("root recorded %+v", got[0])
	}
	if _, _, err := tr.Start(context.Background(), ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("name: %v", err)
	}
}

func TestLogDropsTheOldest(t *testing.T) {
	lg, err := NewLog(2, func() time.Time { return time.Unix(0, 0).UTC() })
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Write("info", "up"); err != nil {
		t.Fatal(err)
	}
	if err := lg.Write("error", "down"); err != nil {
		t.Fatal(err)
	}
	if err := lg.Write("info", "back"); err != nil {
		t.Fatal(err)
	}
	if err := lg.Write("warn", "no"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("level: %v", err)
	}
	got := lg.Events()
	if len(got) != 2 || got[0].Message != "down" || got[1].Message != "back" {
		t.Fatalf("%+v", got)
	}
}

func TestHandlerServesTheScrape(t *testing.T) {
	reg := New()
	c, err := reg.Counter("requests", "Handled requests.")
	if err != nil {
		t.Fatal(err)
	}
	c.Add(1)
	tr, err := NewTracer(TraceConfig{
		Now: func() time.Time { return time.Unix(1, 0).UTC() },
		NewID: func() string {
			return "abc"
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, span, err := tr.Start(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	span.End()
	lg, err := NewLog(0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Write("info", "up"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(reg, tr, lg))
	t.Cleanup(srv.Close)
	if got := body(t, srv.URL+"/metrics"); !strings.Contains(got, "requests 1\n") {
		t.Fatalf("metrics %q", got)
	}
	res, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.Header.Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("type %q", res.Header.Get("Content-Type"))
	}
	if got := body(t, srv.URL+"/traces"); got != "abc abc - demo ok 0\n" {
		t.Fatalf("traces %q", got)
	}
	if got := body(t, srv.URL+"/events"); got != "info up\n" {
		t.Fatalf("events %q", got)
	}
	if got := body(t, srv.URL+"/health"); got != "ok\n" {
		t.Fatalf("health %q", got)
	}
	res, err = http.Get(srv.URL + "/missing")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", res.StatusCode)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestConcurrentUpdates(t *testing.T) {
	reg := New()
	c, err := reg.Counter("requests", "Handled requests.")
	if err != nil {
		t.Fatal(err)
	}
	g, err := reg.Gauge("inflight", "In flight.")
	if err != nil {
		t.Fatal(err)
	}
	h, err := reg.Histogram("latency_ms", "Latency.", []float64{10})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTracer(TraceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Add(1)
			if err := g.Add(1); err != nil {
				t.Errorf("gauge: %v", err)
			}
			if err := h.Observe(1); err != nil {
				t.Errorf("histogram: %v", err)
			}
			_, span, err := tr.Start(context.Background(), "work")
			if err != nil {
				t.Errorf("span: %v", err)
				return
			}
			span.End()
		}()
	}
	wg.Wait()
	if c.Value() != 100 || g.Value() != 100 || h.Count() != 100 || len(tr.Spans()) != 100 {
		t.Fatalf("counter %d gauge %v hist %d spans %d", c.Value(), g.Value(), h.Count(), len(tr.Spans()))
	}
}

func body(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s status %d %s", url, res.StatusCode, buf)
	}
	return string(buf)
}
