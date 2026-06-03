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
