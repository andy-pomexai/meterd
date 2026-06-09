package aggregate

import (
	"context"
	"errors"
	"testing"

	"meterd/internal/store"
)

// fakeSink records what was flushed and can be told to fail.
type fakeSink struct {
	totals map[rollupKey]store.DailyUsage
	writes int
	err    error
}

func newFakeSink() *fakeSink {
	return &fakeSink{totals: make(map[rollupKey]store.DailyUsage)}
}

func (f *fakeSink) AddDailyUsage(_ context.Context, deltas []store.DailyUsage) error {
	if f.err != nil {
		return f.err
	}
	f.writes++
	for _, d := range deltas {
		k := rollupKey{d.TenantID, d.BillingDay, d.Model}
		u := f.totals[k]
		u.Requests += d.Requests
		u.InputTokens += d.InputTokens
		u.OutputTokens += d.OutputTokens
		u.CostMicros += d.CostMicros
		f.totals[k] = u
	}
	return nil
}

func event(tenant, day, model string, in, out, cost int64) store.Event {
	return store.Event{TenantID: tenant, BillingDay: day, Model: model, InputTokens: in, OutputTokens: out, CostMicros: cost}
}

func TestFlushWritesAccumulatedUsage(t *testing.T) {
	sink := newFakeSink()
	agg := New(sink)

	agg.Record(event("t1", "2026-10-14", "atlas-mini", 100, 10, 21))
	agg.Record(event("t1", "2026-10-14", "atlas-mini", 200, 20, 42))
	agg.Record(event("t1", "2026-10-15", "atlas-mini", 5, 5, 4))
	agg.Record(event("t2", "2026-10-14", "atlas-large", 1000, 0, 2500))

	if err := agg.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := map[rollupKey]store.DailyUsage{
		{"t1", "2026-10-14", "atlas-mini"}:  {Requests: 2, InputTokens: 300, OutputTokens: 30, CostMicros: 63},
		{"t1", "2026-10-15", "atlas-mini"}:  {Requests: 1, InputTokens: 5, OutputTokens: 5, CostMicros: 4},
		{"t2", "2026-10-14", "atlas-large"}: {Requests: 1, InputTokens: 1000, OutputTokens: 0, CostMicros: 2500},
	}
	if len(sink.totals) != len(want) {
		t.Fatalf("flushed %d rows, want %d", len(sink.totals), len(want))
	}
	for k, w := range want {
		if got := sink.totals[k]; got != w {
			t.Errorf("%v = %+v, want %+v", k, got, w)
		}
	}
}

func TestFlushDoesNotWriteTwice(t *testing.T) {
	sink := newFakeSink()
	agg := New(sink)
	agg.Record(event("t1", "2026-10-14", "atlas-mini", 100, 10, 21))

	for range 3 {
		if err := agg.Flush(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if sink.writes != 1 {
		t.Errorf("sink was written %d times, want 1", sink.writes)
	}
	if got := sink.totals[rollupKey{"t1", "2026-10-14", "atlas-mini"}].Requests; got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

func TestFailedFlushIsRetried(t *testing.T) {
	sink := newFakeSink()
	agg := New(sink)
	agg.Record(event("t1", "2026-10-14", "atlas-mini", 100, 10, 21))

	sink.err = errors.New("database is unavailable")
	if err := agg.Flush(context.Background()); err == nil {
		t.Fatal("Flush should report the sink's error")
	}

	// More usage arrives while the database is down.
	agg.Record(event("t1", "2026-10-14", "atlas-mini", 200, 20, 42))

	sink.err = nil
	if err := agg.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := sink.totals[rollupKey{"t1", "2026-10-14", "atlas-mini"}]
	want := store.DailyUsage{Requests: 2, InputTokens: 300, OutputTokens: 30, CostMicros: 63}
	if got != want {
		t.Errorf("after retry: %+v, want %+v", got, want)
	}
}
