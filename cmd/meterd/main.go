// Command meterd is the usage metering service for the internal model gateway.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"meterd/internal/server"
	"meterd/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "meterd.db", "path to the SQLite database")
	flushInterval := flag.Duration("flush-interval", 5*time.Second, "how often the usage rollup is written")
	rebuild := flag.String("rebuild", "", "recompute daily_usage from events for billing days FROM:TO (YYYY-MM-DD:YYYY-MM-DD), then exit")
	flag.Parse()

	if *rebuild != "" {
		if err := runRebuild(*dbPath, *rebuild); err != nil {
			log.Fatalf("rebuild: %v", err)
		}
		return
	}

	srv, err := server.New(server.Config{DBPath: *dbPath, FlushInterval: *flushInterval})
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	httpSrv := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}

	go func() {
		log.Printf("meterd listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	// Let in-flight requests finish before the final flush.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err := srv.Close(); err != nil {
		log.Printf("close: %v", err)
	}
}

// runRebuild recomputes the rollup for a FROM:TO range of billing days. Run it
// with ingestion stopped: usage that replicas still hold in memory would be
// added on top of the rebuilt rows.
func runRebuild(dbPath, span string) error {
	from, to, ok := strings.Cut(span, ":")
	if !ok {
		return errors.New("range must be FROM:TO, for example 2026-10-01:2026-10-31")
	}
	for _, day := range []string{from, to} {
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			return err
		}
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.RebuildDailyUsage(context.Background(), from, to)
}
