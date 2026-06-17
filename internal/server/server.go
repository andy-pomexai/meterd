// Package server wires meterd together.
package server

import (
	"context"
	"net/http"
	"time"

	"meterd/internal/aggregate"
	"meterd/internal/api"
	"meterd/internal/ingest"
	"meterd/internal/pricing"
	"meterd/internal/store"
)

// Config is what a meterd replica needs to start.
type Config struct {
	// DBPath is the SQLite database file. It is created if it does not exist.
	DBPath string
	// FlushInterval is how often the usage rollup is written. Defaults to 5s.
	FlushInterval time.Duration
}

// Server is one meterd replica.
type Server struct {
	store *store.Store
	mux   *http.ServeMux

	stopFlusher context.CancelFunc
	flusherDone chan struct{}
}

// New opens the database, starts the background flusher and returns a server
// that is ready to handle requests.
func New(cfg Config) (*Server, error) {
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	agg := aggregate.New(st)
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		store:       st,
		mux:         http.NewServeMux(),
		stopFlusher: cancel,
		flusherDone: make(chan struct{}),
	}
	go func() {
		defer close(s.flusherDone)
		agg.Run(ctx, cfg.FlushInterval)
	}()

	usage := api.NewHandler(st)
	s.mux.Handle("POST /v1/events", ingest.NewHandler(st, pricing.Default(), agg))
	s.mux.HandleFunc("GET /v1/usage", usage.List)
	s.mux.HandleFunc("GET /v1/usage/export", usage.Export)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return s, nil
}

// Handler returns the HTTP handler for this replica.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Close stops the flusher, which writes out whatever usage is still pending,
// and closes the database. Stop sending requests before calling it.
func (s *Server) Close() error {
	s.stopFlusher()
	<-s.flusherDone
	return s.store.Close()
}
