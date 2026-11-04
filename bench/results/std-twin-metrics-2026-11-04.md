# std/metrics against prometheus/client_golang v1.23.2 (2026-11-04)

Twin bench/perf/metrics (go/ module with client_golang v1.23.2 and
prometheus/common v0.73.0): `counter` is 3,000,000 increments of a counter
with one label over 50 series (`WithLabelValues(...).Inc()` against
`c.inc([name])`), `histogram` 2,000,000 observations over 20 series with
the default latency buckets, `scrape` renders 500 counter series and 50
histogram series 300 times. linux/arm64 host 1, 7 rounds (n=6), Go first,
Xo / Go:

| workload | Go | Xo | Xo / Go | range |
|---|---|---|---|---|
| counter, first run | 71.85 | 314.5 | 4.378 | 4.117-4.649 |
| counter, one label is its own key and one bucket per observation | 65.23 | 291.4 | 4.468 | 4.164-4.573 |
| counter, series_of's closure captures fields, not the Family | 70.6 | 287.3 | 4.070 | 4.009-4.180 |
| histogram, first run | 73.09 | 256.4 | 3.508 | 3.414-3.546 |
| histogram, after the bucket change | 72.25 | 242.5 | 3.357 | 3.266-3.385 |
| histogram, after the capture fix | 74.1 | 241.3 | 3.256 | 3.213-3.274 |
| scrape | 108.4 | 51.49 | 0.475 | 0.469-0.482 |

Xo's own time fell 9% (counter) and 6% (histogram) over the three steps;
the ratio did not move much because Go's time also moved 8% between runs.
That is a small win and the gap stays: about 96 ns an increment against
Go's 23. The scrape is twice as fast as Go's.

What the changes were: series_of keyed a one label series by a copy of its
value (join of one element), now by the value; histogram observe added to
every bucket whose bound is at or above the value (up to 11 atomic adds),
now to the first one, as client_golang's findBucket does, and the scrape
adds them up (the text is unchanged, golden metrics_buckets); the closure
that makes a series captured the 13 word Family and shared each of its
heap fields on every call (xo vet --cost XO0910), it captures the three
fields it reads now.

Where the rest goes (Mac XO_PROF, then host 1 at one thread): the call
makes a List literal (alloc and free) for the labels, a closure for
with_read (alloc), the read lock, a hash and a compare, then a clone of
the six field Series out of the map (six reference count operations and six
more to drop) and the CAS; no one function holds more than 11%.

Next experiments, ranked:
1. A cached child, as client_golang's `WithLabelValues` returns one:
   `c.labels(["a"])` giving a handle that holds the series' atomic, so the
   hot loop does no label list, key or lookup (an API addition: spec
   std.md and an ADR first). Expected: counter near Go's, as the handle
   is one atomic add.
2. Without an API change: store the counter's cell apart from the Series
   (Family.cells: Mutex[Map[Str, Atomic[Int]]]) so a hit clones one
   atomic, not six fields.
3. A map read that returns a borrowed view (Lang 1; Map.get clones the
   value), which would help every Mutex[Map[..]] cache in std.
