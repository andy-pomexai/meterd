package store

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "meterd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.UpsertTenant(context.Background(), Tenant{ID: "t1", Name: "Tenant One", DiscountBps: 250}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meterd.db")
	for range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var applied int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
			t.Fatal(err)
		}
		if applied != len(migrations) {
			t.Errorf("%d migrations recorded, want %d", applied, len(migrations))
		}
		s.Close()
	}
}

func TestTenant(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	got, err := s.Tenant(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Tenant{ID: "t1", Name: "Tenant One", DiscountBps: 250}); got != want {
		t.Errorf("Tenant = %+v, want %+v", got, want)
	}
	if _, err := s.Tenant(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown tenant: err = %v, want ErrNotFound", err)
	}
	if err := s.UpsertTenant(ctx, Tenant{ID: "t1", Name: "Tenant One", DiscountBps: 10_001}); err == nil {
		t.Error("a discount above 100% should be rejected")
	}
}

func TestAddDailyUsageAccumulates(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	row := DailyUsage{TenantID: "t1", BillingDay: "2026-10-14", Model: "atlas-mini", Requests: 2, InputTokens: 300, CachedTokens: 120, OutputTokens: 30, CostMicros: 63}
	other := DailyUsage{TenantID: "t1", BillingDay: "2026-10-15", Model: "atlas-mini", Requests: 1, InputTokens: 5, OutputTokens: 5, CostMicros: 4}
	if err := s.AddDailyUsage(ctx, []DailyUsage{row, other}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDailyUsage(ctx, []DailyUsage{row}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDailyUsage(ctx, "t1", "2026-10-01", "2026-10-31", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []DailyUsage{
		{TenantID: "t1", BillingDay: "2026-10-14", Model: "atlas-mini", Requests: 4, InputTokens: 600, CachedTokens: 240, OutputTokens: 60, CostMicros: 126},
		other,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rollup = %+v\nwant     %+v", got, want)
	}
}

func TestListDailyUsageAfterPosition(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	var rows []DailyUsage
	for _, day := range []string{"2026-10-01", "2026-10-02"} {
		for _, model := range []string{"atlas-large", "atlas-mini", "quartz-pro"} {
			rows = append(rows, DailyUsage{TenantID: "t1", BillingDay: day, Model: model, Requests: 1})
		}
	}
	if err := s.AddDailyUsage(ctx, rows); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDailyUsage(ctx, "t1", "2026-10-01", "2026-10-31", &Position{BillingDay: "2026-10-01", Model: "atlas-mini"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 ||
		got[0].BillingDay != "2026-10-01" || got[0].Model != "quartz-pro" ||
		got[1].BillingDay != "2026-10-02" || got[1].Model != "atlas-large" {
		t.Errorf("rows after (2026-10-01, atlas-mini) = %+v", got)
	}
}

func TestRebuildDailyUsage(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	at := time.Date(2026, 10, 14, 3, 0, 0, 0, time.UTC)
	for i, id := range []string{"evt_1", "evt_2", "evt_3"} {
		e := Event{
			EventID: id, TenantID: "t1", Model: "atlas-mini",
			InputTokens: int64(100 * (i + 1)), OutputTokens: 10, CostMicros: int64(20 * (i + 1)),
			OccurredAt: at, ReceivedAt: at, BillingDay: "2026-10-14",
		}
		if err := s.InsertEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	// The rollup has drifted: one row is wrong and one should not exist at all.
	stale := []DailyUsage{
		{TenantID: "t1", BillingDay: "2026-10-14", Model: "atlas-mini", Requests: 99, InputTokens: 1, OutputTokens: 1, CostMicros: 1},
		{TenantID: "t1", BillingDay: "2026-10-14", Model: "quartz-pro", Requests: 7, InputTokens: 7, OutputTokens: 7, CostMicros: 7},
		{TenantID: "t1", BillingDay: "2026-10-20", Model: "atlas-mini", Requests: 5, InputTokens: 5, OutputTokens: 5, CostMicros: 5},
	}
	if err := s.AddDailyUsage(ctx, stale); err != nil {
		t.Fatal(err)
	}

	if err := s.RebuildDailyUsage(ctx, "2026-10-14", "2026-10-14"); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListDailyUsage(ctx, "t1", "2026-10-01", "2026-10-31", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []DailyUsage{
		{TenantID: "t1", BillingDay: "2026-10-14", Model: "atlas-mini", Requests: 3, InputTokens: 600, OutputTokens: 30, CostMicros: 120},
		stale[2], // outside the rebuilt range, left alone
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rollup = %+v\nwant     %+v", got, want)
	}
}
