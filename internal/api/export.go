package api

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"meterd/internal/billing"
	"meterd/internal/store"
)

// exportPageSize is how many rollup rows the export reads from the database at
// a time.
const exportPageSize = 50

var exportHeader = []string{"billing_day", "model", "requests", "input_tokens", "cached_tokens", "output_tokens", "cost_usd"}

// Export serves GET /v1/usage/export?tenant_id=&month=YYYY-MM: a tenant's usage
// for one billing month as CSV, one line per (billing day, model).
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tenantID, month := q.Get("tenant_id"), q.Get("month")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required")
		return
	}
	from, to, err := billing.MonthRange(month)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Read everything before writing anything, so that a database error can
	// still be reported as one instead of producing a truncated file.
	var all []store.DailyUsage
	var after *store.Position
	for {
		rows, next, err := h.page(r.Context(), tenantID, from, to, after, exportPageSize)
		if err != nil {
			log.Printf("api: export usage: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		all = append(all, rows...)
		if next == nil {
			break
		}
		after = next
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="usage-%s.csv"`, month))
	out := csv.NewWriter(w)
	out.Write(exportHeader)
	for _, u := range all {
		out.Write([]string{
			u.BillingDay,
			u.Model,
			strconv.FormatInt(u.Requests, 10),
			strconv.FormatInt(u.InputTokens, 10),
			strconv.FormatInt(u.CachedTokens, 10),
			strconv.FormatInt(u.OutputTokens, 10),
			formatUSD(u.CostMicros),
		})
	}
	out.Flush()
	if err := out.Error(); err != nil {
		log.Printf("api: write export: %v", err)
	}
}

// formatUSD renders an amount in micro-USD as dollars with six decimals.
func formatUSD(micros int64) string {
	return fmt.Sprintf("%d.%06d", micros/1_000_000, micros%1_000_000)
}
