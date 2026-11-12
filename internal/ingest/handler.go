// Package ingest receives usage events from the gateway.
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"meterd/internal/aggregate"
	"meterd/internal/billing"
	"meterd/internal/pricing"
	"meterd/internal/store"
)

const (
	maxBatch          = 500
	maxBodyBytes      = 1 << 20
	maxTokensPerEvent = 10_000_000
	maxClockSkew      = 5 * time.Minute

	// The gateway gives up on a batch after five minutes of retries, so ten
	// minutes covers every redelivery.
	dedupeWindow = 10 * time.Minute
)

// Handler serves POST /v1/events.
type Handler struct {
	store  *store.Store
	prices *pricing.Table
	agg    *aggregate.Aggregator
	seen   *seenCache
	now    func() time.Time
}

// NewHandler returns the ingest handler.
func NewHandler(st *store.Store, prices *pricing.Table, agg *aggregate.Aggregator) *Handler {
	return &Handler{
		store:  st,
		prices: prices,
		agg:    agg,
		seen:   newSeenCache(dedupeWindow),
		now:    time.Now,
	}
}

type eventJSON struct {
	EventID      string `json:"event_id"`
	TenantID     string `json:"tenant_id"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	CachedTokens int64  `json:"cached_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	OccurredAt   string `json:"occurred_at"`
}

type batchJSON struct {
	Events []eventJSON `json:"events"`
}

type resultJSON struct {
	Accepted   int `json:"accepted"`
	Duplicates int `json:"duplicates"`
}

// validationError marks a problem with the request itself, as opposed to a
// failure on our side. The gateway does not retry these.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func invalid(format string, args ...any) error {
	return &validationError{msg: fmt.Sprintf(format, args...)}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var batch batchJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&batch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if n := len(batch.Events); n == 0 || n > maxBatch {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("a batch must contain between 1 and %d events", maxBatch))
		return
	}

	ctx := r.Context()
	now := h.now().UTC()

	// Validate and price the whole batch before storing any of it, so that a
	// batch with a bad event in it is rejected as a unit.
	events := make([]store.Event, 0, len(batch.Events))
	tenants := make(map[string]store.Tenant)
	for i, in := range batch.Events {
		e, err := h.prepare(ctx, in, now, tenants)
		if err != nil {
			var bad *validationError
			if errors.As(err, &bad) {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("events[%d]: %v", i, err))
			} else {
				log.Printf("ingest: prepare events[%d]: %v", i, err)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		events = append(events, e)
	}

	var res resultJSON
	for _, e := range events {
		if h.seen.SeenOrAdd(e.EventID, now) {
			// A redelivery. Acknowledge it, or the gateway keeps resending.
			res.Duplicates++
			continue
		}
		if err := h.store.InsertEvent(ctx, e); err != nil {
			// The event is not stored, so the gateway's retry must get through.
			h.seen.Forget(e.EventID)
			log.Printf("ingest: store event %s: %v", e.EventID, err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		h.agg.Record(e)
		res.Accepted++
	}
	writeJSON(w, http.StatusOK, res)
}

// prepare validates one incoming event and turns it into the row to store.
// tenants caches lookups for the duration of a batch.
func (h *Handler) prepare(ctx context.Context, in eventJSON, now time.Time, tenants map[string]store.Tenant) (store.Event, error) {
	switch {
	case in.EventID == "":
		return store.Event{}, invalid("event_id is required")
	case in.TenantID == "":
		return store.Event{}, invalid("tenant_id is required")
	case in.InputTokens < 0 || in.InputTokens > maxTokensPerEvent:
		return store.Event{}, invalid("input_tokens must be between 0 and %d", maxTokensPerEvent)
	case in.CachedTokens < 0 || in.CachedTokens > maxTokensPerEvent:
		return store.Event{}, invalid("cached_tokens must be between 0 and %d", maxTokensPerEvent)
	case in.OutputTokens < 0 || in.OutputTokens > maxTokensPerEvent:
		return store.Event{}, invalid("output_tokens must be between 0 and %d", maxTokensPerEvent)
	case !h.prices.Has(in.Model):
		return store.Event{}, invalid("unknown model %q", in.Model)
	}

	occurred, err := time.Parse(time.RFC3339, in.OccurredAt)
	if err != nil {
		return store.Event{}, invalid("occurred_at must be an RFC 3339 timestamp")
	}
	if occurred.After(now.Add(maxClockSkew)) {
		return store.Event{}, invalid("occurred_at is in the future")
	}

	tenant, ok := tenants[in.TenantID]
	if !ok {
		tenant, err = h.store.Tenant(ctx, in.TenantID)
		if errors.Is(err, store.ErrNotFound) {
			return store.Event{}, invalid("unknown tenant %q", in.TenantID)
		}
		if err != nil {
			return store.Event{}, err
		}
		tenants[in.TenantID] = tenant
	}

	cost, err := h.prices.CostOf(pricing.Usage{
		Model:        in.Model,
		InputTokens:  in.InputTokens,
		CachedTokens: in.CachedTokens,
		OutputTokens: in.OutputTokens,
	})
	if err != nil {
		return store.Event{}, err
	}

	occurred = occurred.UTC()
	return store.Event{
		EventID:      in.EventID,
		TenantID:     tenant.ID,
		Model:        in.Model,
		InputTokens:  in.InputTokens,
		CachedTokens: in.CachedTokens,
		OutputTokens: in.OutputTokens,
		CostMicros:   cost,
		OccurredAt:   occurred,
		ReceivedAt:   now,
		BillingDay:   billing.Day(occurred),
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("ingest: write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
