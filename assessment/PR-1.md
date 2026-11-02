# PR #1: Cached token pricing

**Author:** Devon (drafted with an AI coding assistant)

## Summary

Upstream providers charge less for prompt tokens that are served from their
prompt cache. Today we bill tenants the full input price for those tokens. This
PR passes the saving on: cached tokens are metered separately and billed at the
cached rate.

## Changes

- **Events:** new optional field `cached_tokens` on `POST /v1/events`
  (documented in `docs/events.md`).
- **Pricing:** `Price` gets `CachedInputPerMTok`. New `Table.CostOf(Usage)`
  prices input, cached and output tokens; taking a struct means we can add more
  token classes later without touching every call site. `Cost` is deprecated
  but kept for now.
- **Storage:** migration 4 adds `cached_tokens` to `events` and `daily_usage`.
  The rollup and `-rebuild` carry the new column.
- **API:** `cached_tokens` is returned by `GET /v1/usage` and added to the CSV
  export.
- **Tests:** unit tests for `CostOf`, an ingest test for cached tokens, and the
  export test updated for the new column.

## Compatibility

- Gateways that do not send `cached_tokens` yet keep working: the field
  defaults to 0.
- No behavior change for tenants that do not use caching.
- The migration only adds columns, so it is safe to roll out and to roll back.

## Testing

- `go test ./...` passes ✅
- `go vet ./...` is clean ✅

## Rollout

Merge and deploy meterd first, then enable `cached_tokens` in the gateway
(gateway PR #812).
