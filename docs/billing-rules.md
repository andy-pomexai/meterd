# Billing rules

These rules come from Finance and from the internal service agreement (ISA)
that every business unit signs to use the gateway. Changing any of them means
renegotiating that agreement. It is not an engineering decision.

## 1. Billing day

A billing day is a calendar day in **UTC+8**, for every tenant, wherever the
tenant is located. A request belongs to the billing day in which it completed
(`occurred_at`). A billing month is a calendar month of billing days. (FIN-198)

## 2. Rounding

The cost of every request is rounded **up** to the next whole micro-dollar
(USD 0.000001) when its event is ingested. Daily and monthly amounts are sums
of these per-request amounts and are never rounded again. Business units are
invoiced the monthly sum. (ISA §4.2, FIN-212)

## 3. Discounts

A tenant's agreed discount is expressed in basis points and applies to the
list price of each request, before rounding.

## 4. Exactly once

A request is billed exactly once, no matter how many times the gateway delivers
its event (see [events.md](events.md)).

## 5. One set of numbers

The dashboard, the CSV export and the monthly invoice are all produced from
`daily_usage`. A tenant's invoice for a billing month is the sum of its
`daily_usage` rows for that month.
