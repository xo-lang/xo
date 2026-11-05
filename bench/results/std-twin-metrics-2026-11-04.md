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
| counter, a Series holds its cells in one list (3 fields, not 6) | 68.8 | 252.6 | 3.671 | 3.414-3.710 |
| histogram, the same | 75.82 | 203.2 | 2.681 | 2.615-2.722 |
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

## Series packed into three fields (host 1, 9 rounds)

A Series was six fields (the label values, five atomic cells or lists of
them), so every hit shared and then dropped six heap objects. It is three now
(`values`, `cells`, `q`; cell 0 the value, 1 the count, 2 the sum, 3 and on
the buckets): Xo's own time fell 12% (counter 287.3 to 252.6 ms) and 16%
(histogram 241.3 to 203.2) against the previous step, ratios 4.07 to 3.67
and 3.26 to 2.68.

Profile of the counter loop after it (host 1, one thread, 60,000,000
increments, 510 samples): series_of 58% cumulative, of which the read
closure 18%; obj.free1 9%, obj_str_eq 7% (the key compare in the map),
reference count work (rc_defer 11%, drops of a Str, List[Str], List[Atomic]
about 12%), the mutex read lock about 10% (rlock_body, held_del, runlock,
reader_out), closure environment drop 3%, hash 3%. No single piece is left
to cut: the remaining cost is the shape of the call (a label list, a key, a
closure, a lock, a lookup and a clone for every increment), which is what
the child API of decision 0501 removes.

## Held children (decision 0501, host 1, 6 rounds)

`counterheld` and `histogramheld` repeat the counter and histogram workloads
through `child(values)` handles made once (client_golang: `WithLabelValues`
held).

| Workload | Go ms | Xo ms | Xo / Go | range |
|---|---|---|---|---|
| counter (lookup per call) | 70.71 | 260.8 | 3.688 | 3.51-3.901 |
| counterheld | 4.508 | 4.53 | 1.005 | 0.95-1.071 |
| histogram (lookup per call) | 74.0 | 205.4 | 2.775 | 2.621-2.813 |
| histogramheld | 27.96 | 23.32 | 0.834 | 0.79-0.858 |

A held child is at parity with Go (counter) or faster (histogram); the
lookup per call is the whole gap.
