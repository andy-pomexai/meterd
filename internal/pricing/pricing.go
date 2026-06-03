// Package pricing turns token counts into money.
//
// All amounts are integers in micro-USD (1 USD = 1,000,000 micro-USD). Prices
// are expressed per one million tokens.
package pricing

import (
	"errors"
	"fmt"
)

// ErrUnknownModel is returned when a model has no entry in the price table.
var ErrUnknownModel = errors.New("unknown model")

// Price is the list price of a model, in micro-USD per one million tokens.
type Price struct {
	InputPerMTok  int64
	OutputPerMTok int64
}

// Table is an immutable price list.
type Table struct {
	prices map[string]Price
}

// NewTable returns a price table with the given entries.
func NewTable(prices map[string]Price) *Table {
	cp := make(map[string]Price, len(prices))
	for model, p := range prices {
		cp[model] = p
	}
	return &Table{prices: cp}
}

// Default is the current list price sheet.
func Default() *Table {
	return NewTable(map[string]Price{
		"atlas-large": {InputPerMTok: 2_500_000, OutputPerMTok: 10_000_000},
		"atlas-mini":  {InputPerMTok: 150_000, OutputPerMTok: 600_000},
		"quartz-pro":  {InputPerMTok: 3_000_000, OutputPerMTok: 15_000_000},
	})
}

// Has reports whether the model is on the price list.
func (t *Table) Has(model string) bool {
	_, ok := t.prices[model]
	return ok
}

const (
	tokensPerUnit = 1_000_000 // prices are per million tokens
	bpsScale      = 10_000    // 100% in basis points
)

// Cost returns what a tenant owes for a single request, in micro-USD.
//
// discountBps is the tenant's agreed discount in basis points (250 means
// 2.5%). It is applied before rounding, and the result is rounded up to the
// next whole micro-USD. See docs/billing-rules.md §2 and §3.
//
// Token counts are bounded by ingest validation, so the intermediate product
// stays well inside int64.
func (t *Table) Cost(model string, inputTokens, outputTokens, discountBps int64) (int64, error) {
	p, ok := t.prices[model]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownModel, model)
	}
	list := inputTokens*p.InputPerMTok + outputTokens*p.OutputPerMTok // micro-USD × tokensPerUnit
	return ceilDiv(list*(bpsScale-discountBps), tokensPerUnit*bpsScale), nil
}

// ceilDiv returns ⌈a/b⌉ for a ≥ 0 and b > 0.
func ceilDiv(a, b int64) int64 {
	return (a + b - 1) / b
}
