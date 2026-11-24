package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"forgeobserve"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8095", "listen address")
	flag.Parse()
	if *listen == "" {
		fmt.Fprintln(os.Stderr, "listen is required")
		os.Exit(1)
	}
	reg := forgeobserve.New()
	requests, err := reg.Counter("requests", "Served requests.")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tr, err := forgeobserve.NewTracer(forgeobserve.TraceConfig{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lg, err := forgeobserve.NewLog(0, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	base := forgeobserve.Handler(reg, tr, lg)
	srv := &http.Server{
		Addr:              *listen,
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			ctx, span, err := tr.Start(r.Context(), "request")
			if err == nil {
				defer span.End()
				r = r.WithContext(ctx)
			}
			base.ServeHTTP(w, r)
		}),
	}
	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}()
	fmt.Printf("ForgeObserve listening on %s\n", *listen)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
	shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(shut)
}
