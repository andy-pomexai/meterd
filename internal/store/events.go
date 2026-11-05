package store

import (
	"context"
	"time"
)

// Event is one metered upstream request, as stored. The events table is the
// source of truth for usage; everything else is derived from it.
type Event struct {
	EventID      string
	TenantID     string
	Model        string
	InputTokens  int64
	CachedTokens int64
	OutputTokens int64
	CostMicros   int64
	OccurredAt   time.Time // when the upstream request completed
	ReceivedAt   time.Time // when meterd received the event
	BillingDay   string    // see billing.Day
}

// InsertEvent stores an event.
func (s *Store) InsertEvent(ctx context.Context, e Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (event_id, tenant_id, model, input_tokens, cached_tokens, output_tokens,
		                    cost_micros, occurred_at, received_at, billing_day)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EventID, e.TenantID, e.Model, e.InputTokens, e.CachedTokens, e.OutputTokens,
		e.CostMicros, formatTime(e.OccurredAt), formatTime(e.ReceivedAt), e.BillingDay)
	return err
}

// CountEvents returns the number of stored events for a tenant on a billing day.
func (s *Store) CountEvents(ctx context.Context, tenantID, billingDay string) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE tenant_id = ? AND billing_day = ?`, tenantID, billingDay,
	).Scan(&n)
	return n, err
}
