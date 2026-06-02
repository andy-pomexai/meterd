package store

import (
	"context"
	"database/sql"
	"errors"
)

// Tenant is a business unit of the company. Tenants are provisioned by the control
// plane, which writes this table directly.
type Tenant struct {
	ID   string
	Name string
	// DiscountBps is the tenant's agreed discount in basis points.
	DiscountBps int64
}

// Tenant returns the tenant with the given id, or ErrNotFound.
func (s *Store) Tenant(ctx context.Context, id string) (Tenant, error) {
	var t Tenant
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, discount_bps FROM tenants WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.DiscountBps)
	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	return t, err
}

// UpsertTenant creates the tenant or updates its name and discount.
func (s *Store) UpsertTenant(ctx context.Context, t Tenant) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, discount_bps) VALUES (?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, discount_bps = excluded.discount_bps`,
		t.ID, t.Name, t.DiscountBps)
	return err
}
