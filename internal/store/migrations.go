package store

import (
	"context"
	"fmt"
	"time"
)

// migrations are applied in order, once each. Never edit a migration that has
// shipped; add a new one.
var migrations = []string{
	// 1: tenants and the raw events reported by the gateway
	`CREATE TABLE tenants (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		discount_bps INTEGER NOT NULL DEFAULT 0 CHECK (discount_bps BETWEEN 0 AND 10000)
	);
	CREATE TABLE events (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id      TEXT NOT NULL,
		tenant_id     TEXT NOT NULL REFERENCES tenants (id),
		model         TEXT NOT NULL,
		input_tokens  INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		cost_micros   INTEGER NOT NULL,
		occurred_at   TEXT NOT NULL,
		received_at   TEXT NOT NULL,
		billing_day   TEXT NOT NULL
	);
	CREATE INDEX events_tenant_day ON events (tenant_id, billing_day);`,

	// 2: per-day rollup served by the usage API
	`CREATE TABLE daily_usage (
		tenant_id     TEXT NOT NULL REFERENCES tenants (id),
		billing_day   TEXT NOT NULL,
		model         TEXT NOT NULL,
		requests      INTEGER NOT NULL,
		input_tokens  INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		cost_micros   INTEGER NOT NULL,
		PRIMARY KEY (tenant_id, billing_day, model)
	);`,

	// 3: support looks single events up by id when a tenant disputes a request
	`CREATE INDEX events_event_id ON events (event_id);`,
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}
	for v := current + 1; v <= len(migrations); v++ {
		if err := s.applyMigration(ctx, v); err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, version int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, migrations[version-1]); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		version, formatTime(time.Now())); err != nil {
		return err
	}
	return tx.Commit()
}
