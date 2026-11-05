package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"meterd/internal/aggregate"
	"meterd/internal/pricing"
	"meterd/internal/store"
)

func newTestHandler(t *testing.T) (*Handler, *store.Store, *aggregate.Aggregator) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "meterd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.UpsertTenant(context.Background(), store.Tenant{ID: "t_test", Name: "Test Tenant"}); err != nil {
		t.Fatal(err)
	}
	agg := aggregate.New(st)
	h := NewHandler(st, pricing.Default(), agg)
	h.now = func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) }
	return h, st, agg
}

func post(t *testing.T, h http.Handler, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body)))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %q", rec.Body.String())
	}
	return rec.Code, out
}

const threeEvents = `{"events":[
	{"event_id":"evt_1","tenant_id":"t_test","model":"atlas-mini","input_tokens":1200,"output_tokens":350,"occurred_at":"2026-10-14T03:00:00Z"},
	{"event_id":"evt_2","tenant_id":"t_test","model":"atlas-mini","input_tokens":800,"output_tokens":50,"occurred_at":"2026-10-14T03:00:05Z"},
	{"event_id":"evt_3","tenant_id":"t_test","model":"atlas-large","input_tokens":1000,"output_tokens":200,"occurred_at":"2026-10-13T16:30:00Z"}
]}`

func TestIngestStoresAndRollsUp(t *testing.T) {
	h, st, agg := newTestHandler(t)
	ctx := context.Background()

	code, res := post(t, h, threeEvents)
	if code != http.StatusOK || res["accepted"] != float64(3) || res["duplicates"] != float64(0) {
		t.Fatalf("POST = %d %v, want 200 with 3 accepted", code, res)
	}
	if err := agg.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListDailyUsage(ctx, "t_test", "2026-10-01", "2026-10-31", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	// evt_3 completed at 00:30 on the 14th in billing time, so it belongs to the 14th.
	want := []store.DailyUsage{
		{TenantID: "t_test", BillingDay: "2026-10-14", Model: "atlas-large", Requests: 1, InputTokens: 1000, OutputTokens: 200, CostMicros: 4500},
		{TenantID: "t_test", BillingDay: "2026-10-14", Model: "atlas-mini", Requests: 2, InputTokens: 2000, OutputTokens: 400, CostMicros: 540},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rollup = %+v\nwant     %+v", got, want)
	}
}

func TestIngestRejectsBadBatches(t *testing.T) {
	good := `{"event_id":"evt_ok","tenant_id":"t_test","model":"atlas-mini","input_tokens":1,"output_tokens":1,"occurred_at":"2026-10-14T03:00:00Z"}`
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"not JSON", `{"events":`, "invalid JSON"},
		{"empty batch", `{"events":[]}`, "between 1 and 500"},
		{"missing event_id", `{"events":[` + good + `,{"tenant_id":"t_test","model":"atlas-mini","occurred_at":"2026-10-14T03:00:00Z"}]}`, "events[1]: event_id is required"},
		{"negative tokens", `{"events":[{"event_id":"e","tenant_id":"t_test","model":"atlas-mini","input_tokens":-1,"occurred_at":"2026-10-14T03:00:00Z"}]}`, "input_tokens"},
		{"negative cached tokens", `{"events":[{"event_id":"e","tenant_id":"t_test","model":"atlas-mini","cached_tokens":-1,"occurred_at":"2026-10-14T03:00:00Z"}]}`, "cached_tokens"},
		{"unknown model", `{"events":[{"event_id":"e","tenant_id":"t_test","model":"no-such-model","occurred_at":"2026-10-14T03:00:00Z"}]}`, "unknown model"},
		{"unknown tenant", `{"events":[{"event_id":"e","tenant_id":"t_nobody","model":"atlas-mini","occurred_at":"2026-10-14T03:00:00Z"}]}`, "unknown tenant"},
		{"bad timestamp", `{"events":[{"event_id":"e","tenant_id":"t_test","model":"atlas-mini","occurred_at":"yesterday"}]}`, "RFC 3339"},
		{"future timestamp", `{"events":[{"event_id":"e","tenant_id":"t_test","model":"atlas-mini","occurred_at":"2026-10-14T12:10:00Z"}]}`, "future"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, st, _ := newTestHandler(t)
			code, res := post(t, h, tt.body)
			msg, _ := res["error"].(string)
			if code != http.StatusBadRequest || !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("POST = %d %q, want 400 mentioning %q", code, msg, tt.wantMsg)
			}
			// Nothing from a rejected batch may be stored, including its valid events.
			n, err := st.CountEvents(context.Background(), "t_test", "2026-10-14")
			if err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Errorf("%d events were stored from a rejected batch", n)
			}
		})
	}
}

func TestIngestIgnoresRedelivery(t *testing.T) {
	h, st, agg := newTestHandler(t)
	ctx := context.Background()

	if code, res := post(t, h, threeEvents); code != http.StatusOK {
		t.Fatalf("first delivery = %d %v", code, res)
	}
	// The gateway did not see our answer in time and sends the batch again.
	code, res := post(t, h, threeEvents)
	if code != http.StatusOK || res["accepted"] != float64(0) || res["duplicates"] != float64(3) {
		t.Fatalf("redelivery = %d %v, want 200 with 3 duplicates", code, res)
	}
	if err := agg.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	n, err := st.CountEvents(ctx, "t_test", "2026-10-14")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("stored %d events, want 3", n)
	}
	rows, err := st.ListDailyUsage(ctx, "t_test", "2026-10-14", "2026-10-14", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	var requests int64
	for _, r := range rows {
		requests += r.Requests
	}
	if requests != 3 {
		t.Errorf("rollup counts %d requests, want 3", requests)
	}
}

func TestIngestCachedTokens(t *testing.T) {
	h, st, agg := newTestHandler(t)
	ctx := context.Background()

	code, res := post(t, h, `{"events":[
		{"event_id":"evt_c1","tenant_id":"t_test","model":"atlas-large","input_tokens":1000,"cached_tokens":800,"output_tokens":200,"occurred_at":"2026-10-14T03:00:00Z"},
		{"event_id":"evt_c2","tenant_id":"t_test","model":"atlas-large","input_tokens":2000,"output_tokens":100,"occurred_at":"2026-10-14T03:00:05Z"}
	]}`)
	if code != http.StatusOK || res["accepted"] != float64(2) {
		t.Fatalf("POST = %d %v, want 200 with 2 accepted", code, res)
	}
	if err := agg.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListDailyUsage(ctx, "t_test", "2026-10-14", "2026-10-14", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []store.DailyUsage{
		{TenantID: "t_test", BillingDay: "2026-10-14", Model: "atlas-large", Requests: 2, InputTokens: 3000, CachedTokens: 800, OutputTokens: 300, CostMicros: 11500},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rollup = %+v\nwant     %+v", got, want)
	}
}
