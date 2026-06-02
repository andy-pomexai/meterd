// Package store is meterd's persistence layer.
//
// Production runs on a shared Postgres instance. This repository uses SQLite so
// that the service can be run and tested without any infrastructure; treat the
// database as shared between replicas either way.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // database/sql driver
)

// ErrNotFound is returned when a row that was asked for does not exist.
var ErrNotFound = errors.New("not found")

// Store gives access to the meterd database.
type Store struct {
	db *sql.DB
}

// Open opens the database at path, creating it if needed, and applies any
// pending migrations.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// SQLite allows one writer at a time. A small pool makes writers queue in
	// Go instead of retrying against the file lock.
	db.SetMaxOpenConns(4)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

// Close releases the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// formatTime is how timestamps are stored: RFC 3339, UTC, second precision.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
