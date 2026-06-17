package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meterd/internal/store"
)

// TestEndToEnd sends events to a replica, shuts it down cleanly and checks
// that a fresh replica serves the usage.
func TestEndToEnd(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "meterd.db")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTenant(context.Background(), store.Tenant{ID: "t_test", Name: "Test Tenant", DiscountBps: 0}); err != nil {
		t.Fatal(err)
	}
	st.Close()

	// A long flush interval: the usage must be written by Close, not by the ticker.
	first, err := New(Config{DBPath: dbPath, FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	occurred := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	body := `{"events":[
		{"event_id":"evt_a","tenant_id":"t_test","model":"quartz-pro","input_tokens":2000,"output_tokens":100,"occurred_at":"` + occurred + `"},
		{"event_id":"evt_b","tenant_id":"t_test","model":"quartz-pro","input_tokens":1000,"output_tokens":300,"occurred_at":"` + occurred + `"}
	]}`
	rec := httptest.NewRecorder()
	first.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/events = %d %s", rec.Code, rec.Body)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := New(Config{DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	rec = httptest.NewRecorder()
	second.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/usage?tenant_id=t_test&from=2000-01-01&to=2999-12-31", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/usage = %d %s", rec.Code, rec.Body)
	}
	var resp struct {
		Data []store.DailyUsage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(resp.Data), resp.Data)
	}
	// 3000 input at $3/M + 400 output at $15/M = 9000 + 6000 micro-USD.
	got := resp.Data[0]
	if got.Requests != 2 || got.InputTokens != 3000 || got.OutputTokens != 400 || got.CostMicros != 15_000 {
		t.Errorf("usage = %+v", got)
	}
}
