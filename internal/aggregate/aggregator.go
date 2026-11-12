// Package aggregate keeps the per-day usage rollup up to date.
//
// Writing the rollup on every request made daily_usage the hottest table in
// the database, so usage is accumulated in memory and written out in a single
// transaction every few seconds. The events table stays the source of truth;
// the rollup lags it by at most one flush interval.
package aggregate

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"meterd/internal/store"
)

// Sink is where accumulated usage is written. *store.Store implements it.
type Sink interface {
	AddDailyUsage(ctx context.Context, deltas []store.DailyUsage) error
}

type rollupKey struct {
	tenantID   string
	billingDay string
	model      string
}

// Aggregator accumulates usage per (tenant, billing day, model) and flushes it
// to a Sink. It is safe for concurrent use.
type Aggregator struct {
	sink Sink

	mu      sync.Mutex
	pending map[rollupKey]store.DailyUsage
}

// New returns an Aggregator that writes to sink.
func New(sink Sink) *Aggregator {
	return &Aggregator{sink: sink, pending: make(map[rollupKey]store.DailyUsage)}
}

// Record adds one stored event to the pending usage.
func (a *Aggregator) Record(e store.Event) {
	k := rollupKey{tenantID: e.TenantID, billingDay: e.BillingDay, model: e.Model}

	a.mu.Lock()
	defer a.mu.Unlock()
	u := a.pending[k]
	u.TenantID, u.BillingDay, u.Model = e.TenantID, e.BillingDay, e.Model
	u.Requests++
	u.InputTokens += e.InputTokens
	u.CachedTokens += e.CachedTokens
	u.OutputTokens += e.OutputTokens
	u.CostMicros += e.CostMicros
	a.pending[k] = u
}

// Flush writes the pending usage to the sink. If the write fails, the pending
// usage is kept and goes out with the next flush.
func (a *Aggregator) Flush(ctx context.Context) error {
	a.mu.Lock()
	deltas := make([]store.DailyUsage, 0, len(a.pending))
	for _, u := range a.pending {
		deltas = append(deltas, u)
	}
	a.mu.Unlock()

	if len(deltas) == 0 {
		return nil
	}

	// The write happens outside the lock so that ingest never waits on the
	// database.
	if err := a.sink.AddDailyUsage(ctx, deltas); err != nil {
		return fmt.Errorf("flush %d rollup rows: %w", len(deltas), err)
	}

	a.mu.Lock()
	a.pending = make(map[rollupKey]store.DailyUsage)
	a.mu.Unlock()
	return nil
}

// Run flushes every interval until ctx is cancelled, then flushes one last
// time so that a clean shutdown loses nothing.
func (a *Aggregator) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := a.Flush(ctx); err != nil {
				log.Printf("aggregate: %v", err)
			}
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := a.Flush(flushCtx); err != nil {
				log.Printf("aggregate: final flush: %v", err)
			}
			cancel()
			return
		}
	}
}
