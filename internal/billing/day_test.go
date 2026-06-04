package billing

import (
	"testing"
	"time"
)

func TestDay(t *testing.T) {
	tests := []struct {
		at   string
		want string
	}{
		{"2026-10-14T03:00:00Z", "2026-10-14"},
		{"2026-10-14T15:59:59Z", "2026-10-14"},
		{"2026-10-14T16:00:00Z", "2026-10-15"}, // midnight in UTC+8
		{"2026-10-31T16:00:00Z", "2026-11-01"},
		{"2026-10-14T23:30:00-07:00", "2026-10-15"}, // the sender's own offset does not matter
	}
	for _, tt := range tests {
		at, err := time.Parse(time.RFC3339, tt.at)
		if err != nil {
			t.Fatal(err)
		}
		if got := Day(at); got != tt.want {
			t.Errorf("Day(%s) = %s, want %s", tt.at, got, tt.want)
		}
	}
}

func TestMonthRange(t *testing.T) {
	tests := []struct {
		month    string
		from, to string
	}{
		{"2026-10", "2026-10-01", "2026-10-31"},
		{"2026-02", "2026-02-01", "2026-02-28"},
		{"2028-02", "2028-02-01", "2028-02-29"},
	}
	for _, tt := range tests {
		from, to, err := MonthRange(tt.month)
		if err != nil {
			t.Fatalf("MonthRange(%s): %v", tt.month, err)
		}
		if from != tt.from || to != tt.to {
			t.Errorf("MonthRange(%s) = %s..%s, want %s..%s", tt.month, from, to, tt.from, tt.to)
		}
	}
	if _, _, err := MonthRange("2026-13"); err == nil {
		t.Error("MonthRange(2026-13) should fail")
	}
}
