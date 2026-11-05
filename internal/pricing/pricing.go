// Package pricing turns token counts into money.
//
// All amounts are integers in micro-USD (1 USD = 1,000,000 micro-USD). Prices
// are expressed per one million tokens.
package pricing

import (
	"errors"
	"fmt"
	"math"
)

// ErrUnknownModel is returned when a model has no entry in the price table.
var ErrUnknownModel = errors.New("unknown model")

// Price is the list price of a model, in micro-USD per one million tokens.
type Price struct {
	InputPerMTok       int64
	CachedInputPerMTok int64
	OutputPerMTok      int64
}

// Usage describes the token usage of a single request.
type Usage struct {
	Model        string
	InputTokens  int64
	CachedTokens int64
	OutputTokens int64
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
		"atlas-large": {InputPerMTok: 2_500_000, CachedInputPerMTok: 1_250_000, OutputPerMTok: 10_000_000},
		"atlas-mini":  {InputPerMTok: 150_000, CachedInputPerMTok: 75_000, OutputPerMTok: 600_000},
		"quartz-pro":  {InputPerMTok: 3_000_000, CachedInputPerMTok: 300_000, OutputPerMTok: 15_000_000},
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

// CostOf returns the cost of a single request, in micro-USD.
//
// Each class of tokens (input, cached input, output) is priced at its own rate
// and the results are summed. New token classes can be added to Usage without
// changing the signature.
func (t *Table) CostOf(u Usage) (int64, error) {
	p, ok := t.prices[u.Model]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownModel, u.Model)
	}
	cost := perMTok(u.InputTokens, p.InputPerMTok) +
		perMTok(u.CachedTokens, p.CachedInputPerMTok) +
		perMTok(u.OutputTokens, p.OutputPerMTok)
	return int64(math.Round(cost)), nil
}

// perMTok converts a token count and a per-million-token price into micro-USD.
func perMTok(tokens, pricePerMTok int64) float64 {
	return float64(tokens) / tokensPerUnit * float64(pricePerMTok)
}

// Cost returns what a tenant owes for a single request, in micro-USD.
//
// Deprecated: use CostOf, which also prices cached tokens. Cost is kept for
// the invoice-preview job, which still passes raw token counts.
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
