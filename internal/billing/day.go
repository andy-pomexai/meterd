// Package billing holds the rules that decide which bill a request lands on.
package billing

import (
	"fmt"
	"time"
)

// Zone is the billing time zone. Every tenant is billed on UTC+8 calendar
// days, wherever the tenant is located. See docs/billing-rules.md §1.
var Zone = time.FixedZone("UTC+8", 8*60*60)

// Day returns the billing day (YYYY-MM-DD) that a request completed at t
// belongs to.
func Day(t time.Time) string {
	return t.In(Zone).Format(time.DateOnly)
}

// MonthRange returns the first and last billing day of a month given as
// YYYY-MM.
func MonthRange(month string) (from, to string, err error) {
	first, err := time.Parse("2006-01", month)
	if err != nil {
		return "", "", fmt.Errorf("month must be YYYY-MM: %w", err)
	}
	last := first.AddDate(0, 1, -1)
	return first.Format(time.DateOnly), last.Format(time.DateOnly), nil
}
