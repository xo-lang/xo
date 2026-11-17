# 0694: std/ui query subscriptions, each page sent the part of the model its query asks for

Status: Proposed (xo-4c, 2026-11-17)

## Context

Decision 0664 stage 4c (and 0659 stage 4) left query subscriptions
(partial replication) for models too large to send whole. Until now
every page got the whole model (or, with `redact`, the principal's
whole share of it), and every sync and live notice was about the whole
model. A page's query (tab state, decision 0672) already says what the
page shows: a filter, a room, a date range.

## Decision

- `host(subset: fn(M, url.Values) -> M = None)` and the same option on
  `host_with`: pure, applied after `redact` to every model and view a
  page receives (its state, a sync's `model` and `html`, a live push),
  with the page's query parsed as `url.Values`.
- A host with `subset` sends no log entries (they hold every Msg), as
  with `redact`. The page and each sync carry the digest of what that
  page is shown, now per (principal, query): a sync without Msgs whose
  digest matches answers `same`, and a live connection (which sends its
  query as `q=`) is told only of changes to its query's part.
- The page's root carries `data-xo-subset`; the runtime of such a page
  does not render a query change locally (its model may not hold the new
  query's part) and fetches it with a sync instead.
- The user state step (decision 0672) sees the model as the page shows
  it.

## Consequences

- A page of a large model holds and receives only its part; a change to
  another part costs it nothing (no wake, a `same` sync).
- A subset page's query change is one round trip, not an instant local
  render. Prediction still runs over the part the page holds.
- A Xo `Replica` of a subset host gets snapshots only, as with `redact`.

## Alternatives

- Subscriptions as a separate request kind (a page asks for named
  parts): more API for what the query already says.
- Subset in the App (`App.subset`): the App would then know about URLs
  twice; the host already owns what is sent.
