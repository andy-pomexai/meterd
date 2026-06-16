// Package api serves usage to the dashboard, to tenants and to billing.
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"meterd/internal/store"
)

const (
	defaultPageSize = 100
	maxPageSize     = 500
)

// Handler serves the usage endpoints.
type Handler struct {
	store *store.Store
}

// NewHandler returns the usage API handler.
func NewHandler(st *store.Store) *Handler {
	return &Handler{store: st}
}

type listResponse struct {
	Data       []store.DailyUsage `json:"data"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

// List serves GET /v1/usage?tenant_id=&from=&to=[&limit=][&cursor=].
//
// Rows are ordered by (billing_day, model). When there are more rows than fit
// on a page the response carries next_cursor; passing it back returns the page
// that follows.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tenantID, from, to := q.Get("tenant_id"), q.Get("from"), q.Get("to")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required")
		return
	}
	if !isDay(from) || !isDay(to) {
		writeError(w, http.StatusBadRequest, "from and to must be billing days (YYYY-MM-DD)")
		return
	}

	limit := defaultPageSize
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxPageSize {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and "+strconv.Itoa(maxPageSize))
			return
		}
		limit = n
	}

	var after *store.Position
	if s := q.Get("cursor"); s != "" {
		pos, err := decodeCursor(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		after = &pos
	}

	rows, next, err := h.page(r.Context(), tenantID, from, to, after, limit)
	if err != nil {
		log.Printf("api: list usage: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp := listResponse{Data: rows}
	if resp.Data == nil {
		resp.Data = []store.DailyUsage{}
	}
	if next != nil {
		resp.NextCursor = encodeCursor(*next)
	}
	writeJSON(w, http.StatusOK, resp)
}

// page returns one page of rollup rows and the position to continue from, or
// a nil position if this was the last page.
func (h *Handler) page(ctx context.Context, tenantID, from, to string, after *store.Position, limit int) ([]store.DailyUsage, *store.Position, error) {
	// Ask for one row more than the page holds: if it comes back, there is
	// another page.
	rows, err := h.store.ListDailyUsage(ctx, tenantID, from, to, after, limit+1)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) <= limit {
		return rows, nil, nil
	}
	next := &store.Position{BillingDay: rows[limit].BillingDay, Model: rows[limit].Model}
	return rows[:limit], next, nil
}

// A cursor is an opaque token for a Position.

func encodeCursor(p store.Position) string {
	return base64.RawURLEncoding.EncodeToString([]byte(p.BillingDay + "|" + p.Model))
}

func decodeCursor(s string) (store.Position, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return store.Position{}, err
	}
	day, model, ok := strings.Cut(string(raw), "|")
	if !ok || !isDay(day) || model == "" {
		return store.Position{}, errors.New("malformed cursor")
	}
	return store.Position{BillingDay: day, Model: model}, nil
}

func isDay(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
