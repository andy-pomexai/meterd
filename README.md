# meterd

> **This repository is a take-home exercise.** The company and the systems it
> describes are fictional. If you were sent here as a candidate, start with
> [ASSESSMENT.md](ASSESSMENT.md).

`meterd` is the usage metering service behind the company's internal model
gateway. Teams here do not call LLM providers directly: every request goes
through the gateway, which proxies it to an upstream model provider and reports
one *usage event* to meterd. meterd prices the event, stores it, and maintains
a per-day rollup. Each business unit is a *tenant* and pays for what it uses:
the usage dashboard, the CSV export and the monthly internal invoice run all
read that rollup.

```
gateway ── POST /v1/events ──▶ meterd ──▶ events        one row per request, the source of truth
                                      └─▶ daily_usage   rollup per tenant / billing day / model

dashboard, export, invoicing ◀── GET /v1/usage, GET /v1/usage/export ── meterd
```

## Run it

```
go test ./...
go run ./cmd/meterd -db /tmp/meterd.db      # listens on :8080
```

Go 1.26 or newer. There is nothing else to install: the database is a SQLite
file, accessed through a pure-Go driver (no cgo).

## API

| Endpoint | Purpose |
|---|---|
| `POST /v1/events` | The gateway reports a batch of usage events. See [docs/events.md](docs/events.md). |
| `GET /v1/usage?tenant_id=&from=&to=[&limit=][&cursor=]` | Rollup rows for a range of billing days, ordered by (billing day, model), paginated with a cursor. |
| `GET /v1/usage/export?tenant_id=&month=YYYY-MM` | The same rows for one billing month, as CSV. |
| `GET /healthz` | Liveness. |

## Layout

| Path | What lives there |
|---|---|
| `cmd/meterd` | Entry point, flags, graceful shutdown, the `-rebuild` maintenance command |
| `internal/server` | Wires a replica together |
| `internal/ingest` | `POST /v1/events`: validation, pricing, storing |
| `internal/aggregate` | In-memory accumulation of the rollup and its periodic flush |
| `internal/api` | Usage listing and CSV export |
| `internal/pricing` | Price list and per-request cost |
| `internal/billing` | Billing day and month |
| `internal/store` | Schema, migrations and queries |

## How it runs in production

- **Three replicas** behind an internal load balancer. Any request can reach any
  replica. Replicas are restarted on every deploy (several times a week) and
  whenever the orchestrator reschedules them.
- **One shared database.** Production uses Postgres; SQLite stands in for it in
  this repository.
- **Callers are internal services.** Authentication happens in front of meterd
  and is not part of this codebase. Tenants are provisioned by the control
  plane, which writes the `tenants` table.
- **The rollup lags the events** by up to one flush interval (5 seconds). A
  clean shutdown flushes what is pending. If a replica is killed, the usage it
  had not flushed yet is missing from the rollup, although the events are
  stored; ops repairs that by hand with `meterd -rebuild FROM:TO`, run while
  ingestion is stopped.

## Rules you need to know

- [docs/events.md](docs/events.md): the event schema and how the gateway delivers events.
- [docs/billing-rules.md](docs/billing-rules.md): billing day, rounding, discounts. These are fixed by the internal service agreement.
