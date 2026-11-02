# Data

The data set is small on purpose. Treat the figures as if they were real.

## snapshot.db

A copy of meterd's production database (SQLite) for the **October 2026 billing
month**: the `tenants`, `events` and `daily_usage` tables, as described in
`internal/store/migrations.go`.

The service changes the database it opens. Work on a copy:

```
cp data/snapshot.db /tmp/snapshot.db
sqlite3 /tmp/snapshot.db '.tables'
```

## acme_usage_report_2026-10.csv

What Acme Robotics sent us with METER-142: their own count of requests and
tokens per billing day, from their client-side request log.

## upstream_invoice_2026-10.csv

The October 2026 invoice from the upstream model provider, exported from their
billing portal. All of the gateway's traffic for these three models goes to
this provider.

| Column | Meaning |
|---|---|
| `line` | Line number on the invoice |
| `usage_date` | The **UTC** calendar day on which the usage happened |
| `model` | Model name (same names as ours) |
| `meter` | What is being charged, see below |
| `quantity_tokens` | Tokens on this meter |
| `unit_price_usd_per_mtok` | Price per one million tokens |
| `amount_usd` | Line amount |

Meters, as the provider defines them. For a single request the meters do not
overlap: every prompt token is counted on exactly one of the first three.

| Meter | Meaning |
|---|---|
| `input` | Prompt tokens that were processed normally |
| `cached_input` | Prompt tokens that were read from the prompt cache |
| `cache_write` | Prompt tokens that were written to the prompt cache (only models with explicit caching have this meter) |
| `output` | Completion tokens |

The invoice PDF that came with the export states a **total of USD 1,081.73**.
