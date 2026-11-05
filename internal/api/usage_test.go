package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"meterd/internal/store"
)

func newTestHandler(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "meterd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, id := range []string{"t_test", "t_other"} {
		if err := st.UpsertTenant(context.Background(), store.Tenant{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	return NewHandler(st), st
}

// seed writes one rollup row per (day, model) for October 2026, days 1..days.
func seed(t *testing.T, st *store.Store, tenantID string, days int, models ...string) {
	t.Helper()
	var rows []store.DailyUsage
	for d := 1; d <= days; d++ {
		for _, m := range models {
			rows = append(rows, store.DailyUsage{
				TenantID:     tenantID,
				BillingDay:   fmt.Sprintf("2026-10-%02d", d),
				Model:        m,
				Requests:     int64(d),
				InputTokens:  int64(1000 * d),
				CachedTokens: int64(400 * d),
				OutputTokens: int64(100 * d),
				CostMicros:   int64(1_250_000 * d),
			})
		}
	}
	if err := st.AddDailyUsage(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, h http.HandlerFunc, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
	return rec
}

func list(t *testing.T, h *Handler, query string) listResponse {
	t.Helper()
	rec := get(t, h.List, query)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/usage?%s = %d %s", query, rec.Code, rec.Body)
	}
	var resp listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestListFiltersByTenantAndRange(t *testing.T) {
	h, st := newTestHandler(t)
	seed(t, st, "t_test", 10, "atlas-mini", "atlas-large")
	seed(t, st, "t_other", 10, "atlas-mini")

	resp := list(t, h, "tenant_id=t_test&from=2026-10-03&to=2026-10-04")
	if resp.NextCursor != "" {
		t.Errorf("unexpected next_cursor on a short result")
	}
	var got []string
	for _, r := range resp.Data {
		if r.TenantID != "t_test" {
			t.Fatalf("row for tenant %s leaked into the result", r.TenantID)
		}
		got = append(got, r.BillingDay+" "+r.Model)
	}
	want := []string{"2026-10-03 atlas-large", "2026-10-03 atlas-mini", "2026-10-04 atlas-large", "2026-10-04 atlas-mini"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
	if first := resp.Data[0]; first.Requests != 3 || first.InputTokens != 3000 || first.CostMicros != 3_750_000 {
		t.Errorf("first row = %+v", first)
	}
}

func TestListPaginates(t *testing.T) {
	h, st := newTestHandler(t)
	seed(t, st, "t_test", 7, "atlas-mini")

	var (
		pages  int
		last   string
		cursor string
	)
	for {
		resp := list(t, h, "tenant_id=t_test&from=2026-10-01&to=2026-10-31&limit=3&cursor="+cursor)
		pages++
		if len(resp.Data) == 0 || len(resp.Data) > 3 {
			t.Fatalf("page %d has %d rows, want 1 to 3", pages, len(resp.Data))
		}
		for _, r := range resp.Data {
			// A page continues where the previous one stopped: no repeats, no going back.
			if r.BillingDay <= last {
				t.Fatalf("page %d returned %s after %s", pages, r.BillingDay, last)
			}
			last = r.BillingDay
		}
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
		if pages > 10 {
			t.Fatal("pagination does not terminate")
		}
	}
	if last != "2026-10-07" {
		t.Errorf("paging stopped at %s, want 2026-10-07", last)
	}
}

func TestListRejectsBadRequests(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, query := range []string{
		"from=2026-10-01&to=2026-10-31",
		"tenant_id=t_test&from=2026-10-01",
		"tenant_id=t_test&from=10/01/2026&to=2026-10-31",
		"tenant_id=t_test&from=2026-10-01&to=2026-10-31&limit=0",
		"tenant_id=t_test&from=2026-10-01&to=2026-10-31&limit=501",
		"tenant_id=t_test&from=2026-10-01&to=2026-10-31&cursor=not-a-cursor",
	} {
		if rec := get(t, h.List, query); rec.Code != http.StatusBadRequest {
			t.Errorf("GET /v1/usage?%s = %d, want 400", query, rec.Code)
		}
	}
}

func TestListEmptyResultIsAnEmptyArray(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := get(t, h.List, "tenant_id=t_test&from=2026-10-01&to=2026-10-31")
	if got := rec.Body.String(); got != `{"data":[]}`+"\n" {
		t.Errorf("body = %s", got)
	}
}

func TestExportCSV(t *testing.T) {
	h, st := newTestHandler(t)
	seed(t, st, "t_test", 2, "atlas-mini", "atlas-large")
	seed(t, st, "t_other", 2, "atlas-mini")

	rec := get(t, h.Export, "tenant_id=t_test&month=2026-10")
	if rec.Code != http.StatusOK {
		t.Fatalf("export = %d %s", rec.Code, rec.Body)
	}
	want := "billing_day,model,requests,input_tokens,cached_tokens,output_tokens,cost_usd\n" +
		"2026-10-01,atlas-large,1,1000,400,100,1.250000\n" +
		"2026-10-01,atlas-mini,1,1000,400,100,1.250000\n" +
		"2026-10-02,atlas-large,2,2000,800,200,2.500000\n" +
		"2026-10-02,atlas-mini,2,2000,800,200,2.500000\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("export =\n%s\nwant\n%s", got, want)
	}
}

func TestExportRejectsBadRequests(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, query := range []string{"month=2026-10", "tenant_id=t_test", "tenant_id=t_test&month=October"} {
		if rec := get(t, h.Export, query); rec.Code != http.StatusBadRequest {
			t.Errorf("GET /v1/usage/export?%s = %d, want 400", query, rec.Code)
		}
	}
}
