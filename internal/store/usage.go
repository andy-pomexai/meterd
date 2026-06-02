package store

import (
	"context"
)

// DailyUsage is one row of the usage rollup: what a tenant used of one model
// on one billing day.
type DailyUsage struct {
	TenantID     string `json:"tenant_id"`
	BillingDay   string `json:"billing_day"`
	Model        string `json:"model"`
	Requests     int64  `json:"requests"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CostMicros   int64  `json:"cost_micros"`
}

// Position identifies a row in the (billing_day, model) ordering used by
// ListDailyUsage.
type Position struct {
	BillingDay string
	Model      string
}

// AddDailyUsage adds the given amounts to the rollup, creating rows as needed.
// All of the deltas are applied in one transaction: either every row is
// updated or none is.
func (s *Store) AddDailyUsage(ctx context.Context, deltas []DailyUsage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO daily_usage (tenant_id, billing_day, model, requests, input_tokens, output_tokens, cost_micros)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, billing_day, model) DO UPDATE SET
			requests      = requests + excluded.requests,
			input_tokens  = input_tokens + excluded.input_tokens,
			output_tokens = output_tokens + excluded.output_tokens,
			cost_micros   = cost_micros + excluded.cost_micros`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range deltas {
		if _, err := stmt.ExecContext(ctx, d.TenantID, d.BillingDay, d.Model,
			d.Requests, d.InputTokens, d.OutputTokens, d.CostMicros); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListDailyUsage returns up to limit rollup rows for a tenant with
// from <= billing_day <= to, ordered by (billing_day, model). If after is not
// nil, only rows strictly after that position are returned.
func (s *Store) ListDailyUsage(ctx context.Context, tenantID, from, to string, after *Position, limit int) ([]DailyUsage, error) {
	query := `
		SELECT tenant_id, billing_day, model, requests, input_tokens, output_tokens, cost_micros
		FROM daily_usage
		WHERE tenant_id = ? AND billing_day >= ? AND billing_day <= ?`
	args := []any{tenantID, from, to}
	if after != nil {
		query += ` AND (billing_day, model) > (?, ?)`
		args = append(args, after.BillingDay, after.Model)
	}
	query += ` ORDER BY billing_day, model LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyUsage
	for rows.Next() {
		var u DailyUsage
		if err := rows.Scan(&u.TenantID, &u.BillingDay, &u.Model,
			&u.Requests, &u.InputTokens, &u.OutputTokens, &u.CostMicros); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// RebuildDailyUsage recomputes the rollup for billing days from..to (inclusive)
// from the events table, replacing whatever is there. The rollup is derived
// data; this is the tool for repairing it if it ever drifts from the events.
func (s *Store) RebuildDailyUsage(ctx context.Context, from, to string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM daily_usage WHERE billing_day >= ? AND billing_day <= ?`, from, to); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO daily_usage (tenant_id, billing_day, model, requests, input_tokens, output_tokens, cost_micros)
		SELECT tenant_id, billing_day, model, COUNT(*), SUM(input_tokens), SUM(output_tokens), SUM(cost_micros)
		FROM events
		WHERE billing_day >= ? AND billing_day <= ?
		GROUP BY tenant_id, billing_day, model`, from, to); err != nil {
		return err
	}
	return tx.Commit()
}
