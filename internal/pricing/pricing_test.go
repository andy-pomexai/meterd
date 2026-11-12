package pricing

import (
	"errors"
	"testing"
)

func TestCost(t *testing.T) {
	tbl := Default()
	tests := []struct {
		name        string
		model       string
		in, out     int64
		discountBps int64
		want        int64
	}{
		{"list price per million", "atlas-large", 1_000_000, 1_000_000, 0, 12_500_000},
		{"small request", "atlas-mini", 1200, 350, 0, 390},
		{"discount is applied before rounding", "atlas-large", 1000, 200, 500, 4275},
		{"a fraction of a micro-USD rounds up", "atlas-mini", 1, 0, 0, 1},
		{"rounds up after the discount too", "atlas-mini", 7, 0, 250, 2},
		{"exact amounts are not bumped", "quartz-pro", 2, 0, 0, 6},
		{"zero tokens cost nothing", "quartz-pro", 0, 0, 0, 0},
		{"full discount", "atlas-large", 5000, 5000, 10_000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tbl.Cost(tt.model, tt.in, tt.out, tt.discountBps)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Cost(%s, %d, %d, %d) = %d, want %d", tt.model, tt.in, tt.out, tt.discountBps, got, tt.want)
			}
		})
	}
}

func TestCostUnknownModel(t *testing.T) {
	_, err := Default().Cost("no-such-model", 1, 1, 0)
	if !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("err = %v, want ErrUnknownModel", err)
	}
}

func TestHas(t *testing.T) {
	tbl := NewTable(map[string]Price{"m": {InputPerMTok: 1, OutputPerMTok: 2}})
	if !tbl.Has("m") || tbl.Has("other") {
		t.Fatal("Has reports the wrong models")
	}
}

func TestCostOf(t *testing.T) {
	tbl := Default()
	tests := []struct {
		name  string
		usage Usage
		want  int64
	}{
		{
			name:  "should price input and output tokens",
			usage: Usage{Model: "atlas-large", InputTokens: 1000, OutputTokens: 200},
			want:  4500,
		},
		{
			// 1,000 prompt tokens, 800 of them served from cache, 200 output:
			// 1000 * 2.50 + 800 * 1.25 + 200 * 10.00 = 5500
			name:  "should price cached tokens at the cached rate",
			usage: Usage{Model: "atlas-large", InputTokens: 1000, CachedTokens: 800, OutputTokens: 200},
			want:  5500,
		},
		{
			name:  "should handle a fully cached prompt",
			usage: Usage{Model: "quartz-pro", InputTokens: 10_000, CachedTokens: 10_000, OutputTokens: 0},
			want:  33_000,
		},
		{
			name:  "should price a million tokens of each class",
			usage: Usage{Model: "atlas-mini", InputTokens: 1_000_000, CachedTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  825_000,
		},
		{
			name:  "should return zero for zero usage",
			usage: Usage{Model: "atlas-mini"},
			want:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tbl.CostOf(tt.usage)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("CostOf(%+v) = %d, want %d", tt.usage, got, tt.want)
			}
		})
	}
}

func TestCostOfUnknownModel(t *testing.T) {
	_, err := Default().CostOf(Usage{Model: "no-such-model", InputTokens: 1})
	if !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("err = %v, want ErrUnknownModel", err)
	}
}
