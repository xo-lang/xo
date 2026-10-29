# 0411: std/metrics: scrape-time gauges and collectors, deleting a series, Summary quantiles

Status: Accepted (lead, 2026-10-29: 0.35 std survey rank 10; Xo Lang 2).
Implemented 2026-10-29.

## Context

std/metrics (decision 0325) has counters, gauges and histograms written
by the program. An exporter (the 2026-10-29 survey, rank 10) reads state
when it is scraped (a queue's depth, a pool's connections), drops series
whose subject went away, and reports latency quantiles, as Prometheus'
Go client does with GaugeFunc, Collector, DeleteLabelValues and Summary.

## Decision

- `gauge_func(name, help, f)`, `counter_func(name, help, f)` and
  `collector(name, help, kind, labels, f: fn() -> List[Sample])`: read
  when `text` runs. A collector is one family (Go's Collector may emit
  several; one family per call keeps the registry's name check).
- `remove(values) -> Bool` and `reset()` on every instrument.
- `summary(name, help, labels, objectives, max_age = 10m, age_buckets =
  5)`: CKMS targeted quantiles ported from beorn7/perks (what
  client_golang uses), `age_buckets` streams that each observation
  enters, the head one answering.
- The age buckets rotate at scrape time (option c): `text(now: Time? =
  None)` and `handler_at(clock)` pass a time, and the summary resets the
  buckets whose time ran out before it emits its quantiles. `observe`
  takes no clock and stays pure.

## Consequences

- A summary on a scraped server ages as Go's does; one never scraped with
  a time (`text()`, `handler()`) keeps every observation, which the spec
  states.
- `text()` keeps its signature (the time is an optional argument).

## Alternatives

- (a) No time window: quantiles over every observation since the series
  began or since `reset`. Simple, but a long running server's quantiles
  would stop moving; kept as the behavior without a scrape time.
- (b) A clock captured by `summary` and read in `observe`: rejected. It
  hides a `uses clock` effect behind a pure method (the laundering the
  lead keeps removing; ADR 0404's deferred with_body_file is the same
  problem).
