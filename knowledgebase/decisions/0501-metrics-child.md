# 0501: std/metrics children (a held series, as WithLabelValues)

Status: Accepted (owner, 2026-11-04)

## Context

`c.inc(["a"])` looks the series up on every call: a list literal for the
label values, a key, a read lock, a hash, a clone of the series' fields. On
linux/arm64 host 1 (bench/perf/metrics against prometheus/client_golang
v1.23.2, bench/results/std-twin-metrics-2026-11-04.md) a labelled counter
increment takes about 96 ns against Go's 23 ns (4.07 of Go, histogram
3.26), after the 2026-11-04 changes that cut Xo's time 6 to 9%. Go's code
does the same lookup in `WithLabelValues`, but its callers hold the result
(`v := vec.WithLabelValues("a"); v.Inc()`), so the hot loop is one atomic
add; `Curry` and `With(Labels)` are the other forms.

The lookup can be made cheaper (decision-free work, in progress), but not
free: any call that takes label values must find the series.

## Decision

A child is a handle to one series, made once and used many times:

```
fn (c: Counter) child(values: List[Str]) -> CounterChild
fn (c: CounterChild) inc()
fn (c: CounterChild) add(n: Float)
fn (g: Gauge) child(values: List[Str]) -> GaugeChild      // set, add
fn (h: Histogram) child(values: List[Str]) -> HistogramChild  // observe
fn (s: Summary) child(values: List[Str]) -> SummaryChild  // observe
```

- `child` finds or makes the series (a wrong number of values is the same
  fault as `inc`; past `max_series` the child counts into nothing and the
  drop is counted in `xo_metrics_dropped_total`, as `inc` does).
- A child shares the series' cells; cloning one shares them again; it is safe
  from any task, as `inc` is.
- `remove(values)` and `reset()` detach the series from the exposition; a
  held child keeps counting into the detached series, as client_golang's
  does after `DeleteLabelValues` (documented: take a new child after a
  remove).
- `inc(values)`, `add`, `set`, `observe` stay, now built on `child`.
- Spec: std.md metrics section; the Go backend (std/rt) mirrors the type.

## Consequences

- Expected gain: the hot loop on a child is one compare-and-swap (counter
  increment, a gauge `add`), the histogram one bucket search and three
  atomic adds: no list, key, lock, hash or clone; the counter row of the
  twin should fall from 4.07 to about 1 of client_golang's
  `WithLabelValues(...).Inc()` (which also pays a lookup) and nearer 0.3 of
  a held child's Go loop. To be measured, not assumed, with `xo bench --ab`.
- A new public type per instrument (four), 8 methods. Code that wants the
  fast path changes a loop to hold a child; nothing breaks.
- A stale child after `remove` is a documented trap (as in Go).

## Alternatives

- Make the lookup itself cheaper only (no Series clone on a hit, no list
  literal or closure, a lock only on insert): in progress as a decision-free
  change; its best case is a few tens of ns above Go's `WithLabelValues`.
- A `Labels` map argument (`With(Labels{...})`): slower than values, kept out.
- `Curry`: partial application of a prefix of the labels; can be added later
  on top of `child`.
- Borrowed map reads (Lang 1's Map.get returning a view): helps every
  `Mutex[Map]` cache in std, not this call's allocations.
