# METER-142: Acme Robotics disputes October usage; export is missing a row

**Reporter:** Priya (Platform Support) · **Priority:** High · **Component:** meterd

## Report

Acme Robotics (`t_acme`), one of our business units, disputes its October usage.

**1. Usage is too high.** They compared our dashboard with their own request log
for 14 October (billing day), and we show more than they sent:

|                | requests | input tokens | output tokens |
|----------------|---------:|-------------:|--------------:|
| Acme's own log | 451 | 11,030,718 | 432,628 |
| Our dashboard  | 464 | 11,320,467 | 447,280 |

That is 2.9% more requests than they made. They say other days look
similar, and they sent their numbers for every day of the month:
`data/acme_usage_report_2026-10.csv`. Those numbers are counted per billing day
(UTC+8) from the request log that their own client keeps, one entry per request,
with the same token fields the gateway reports to us.

**2. A row is missing from the export.** Their finance team downloads
`/v1/usage/export` every month. The October file has no line for
`2026-10-17` / `quartz-pro`, although the dashboard shows usage for it.

They want to know **how much they were over-charged for October**, so that
Finance can credit it on their next internal invoice. The October invoice
(USD 776.79) has already been issued.

## Comments

**Marcus (Finance):** Could this be the rounding? We round *every* request up,
and they made about 12,939 requests in October. I have asked before whether we
should round once per invoice instead. If that is where the 3% comes from, let's
just change it.

**Lena (Backend):** I had a quick look before going on leave. For the 14th I
count 466 rows in `events` for Acme, which is neither their number nor the
dashboard's. So either more than one thing is off or I am counting wrong; I did
not get further. `data/snapshot.db` is a copy of prod for the October billing
month, for whoever picks this up.

**Priya (Platform Support):** Acme is one of our three largest tenants and this
is the second time this year that they have disputed an internal invoice.
Whatever number we give them is booked by both finance teams, so it has to
hold.
