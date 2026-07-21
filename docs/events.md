# Usage events

The gateway sends one event per upstream request, after the request has
completed.

## Schema

```
POST /v1/events
{"events": [ { ...event... }, ... ]}        1 to 500 events per batch
```

| Field | Type | Meaning |
|---|---|---|
| `event_id` | string | Identifies the upstream request. Assigned by the gateway; globally unique. The same request always carries the same id, however many times its event is delivered. |
| `tenant_id` | string | The business unit the request was made for. |
| `model` | string | Model name, as on the price list. |
| `input_tokens` | integer | Prompt tokens of the request as reported by the upstream, **including** any tokens the upstream served from its prompt cache. |
| `output_tokens` | integer | Completion tokens. |
| `occurred_at` | RFC 3339 timestamp | When the upstream request completed. |

The response is `200 {"accepted": n, "duplicates": m}`.

## Delivery

Delivery is **at-least-once**.

- The gateway waits 2 seconds for an answer. On a timeout, a connection error
  or any 5xx it sends the same batch again, with backoff, for up to 5 minutes.
  A retry goes through the load balancer like any other request.
- A 4xx is treated as permanent: the batch is dropped and an alert is raised.
  Only answer 4xx when the batch itself is wrong.
- Events that were already ingested must be acknowledged with a 2xx (and
  counted under `duplicates`), otherwise the gateway keeps resending them.
- Batches are not ordered, and events can be late: after a gateway incident,
  `occurred_at` can be hours behind the time of delivery.
