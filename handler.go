package forgeobserve

import (
	"fmt"
	"net/http"
	"strings"
)

// Handler serves GET /metrics, /traces, /events, and /health.
// A nil registry, tracer, or log serves an empty body for that path.
func Handler(reg *Registry, tr *Tracer, lg *Log) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method is not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case "/metrics":
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			if reg != nil {
				_, _ = w.Write([]byte(reg.Text()))
			}
		case "/traces":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			if tr != nil {
				_, _ = w.Write([]byte(tracesText(tr.Spans())))
			}
		case "/events":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			if lg != nil {
				_, _ = w.Write([]byte(eventsText(lg.Events())))
			}
		case "/health":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("ok\n"))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
}

func tracesText(spans []Finished) string {
	var b strings.Builder
	for _, sp := range spans {
		parent := sp.ParentID
		if parent == "" {
			parent = "-"
		}
		fmt.Fprintf(&b, "%s %s %s %s %s %d\n", sp.TraceID, sp.SpanID, parent, sp.Name, sp.Status(), sp.Duration().Microseconds())
	}
	return b.String()
}

func eventsText(events []Event) string {
	var b strings.Builder
	for _, ev := range events {
		fmt.Fprintf(&b, "%s %s\n", ev.Level, ev.Message)
	}
	return b.String()
}
