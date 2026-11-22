# Xo against Go across bench/perf, 2026-11-21

The xo-vs-go survey refreshed at v0.55.0 for the website's benchmark
pages (task #645), in the method of
[xo-vs-go-survey-2026-11-07.md](xo-vs-go-survey-2026-11-07.md). Every
timed row is in [xo-vs-go-survey-2026-11-21.txt](xo-vs-go-survey-2026-11-21.txt).

## Method

- Tree: `git archive 9599fc7d07` (the v0.55.0 commit); xo: the v0.55.0
  linux-arm64 trial archive (`xo 0.55.0 linux/arm64`); Go: `go1.26.11
  linux/arm64`.
- Host 1 (linux/arm64, Graviton), exclusive leases of 30 minutes or
  less: five leases over bench/perf, then two for the twins that need Go
  modules.
- `twincheck -time 5 bench/perf/*`: each case's Xo and Go twins are
  checked to print the same output, then each runs 5 times, alternating,
  and the medians of wall time are compared. Ratio = Xo / Go; below 1 is
  Xo faster.
- Go twins that import modules build with `GOPROXY=off`. Host 1 has no
  module cache, so the first pass failed "building go" for blake2b,
  brotli, chacha, grpc, grpchealth, grpcreflect, otlp and sqlite. A
  module cache filled on the Mac (each twin cross-built for linux/arm64)
  was copied to host 1 and those directories were timed in their own
  leases with `GOMODCACHE` set. The 2026-11-07 survey left its module
  twins out; this one times them.
- Not profiled in this refresh: the "what dominates" column of the
  2026-11-07 survey is not repeated.

## Summary

642 cases timed. Geometric mean of Xo / Go: 0.704 (2026-11-07: 0.724
over 387 cases). 530 cases are faster than Go, 43 are more than 10%
slower, 150 run under 5 ms on both sides (startup dominated; excluded
from the ranking). Over the 492 ranked cases the geometric mean is
0.680; 34 of them are more than 10% slower.

## Top gaps

Ratio is Xo / Go wall time (median of 5), cases where either side runs
5 ms or more.

| # | Case | Ratio | Xo ms | Go ms |
|---|---|---|---|---|
| 1 | ordcmp/str | 2.461 | 124.2 | 50.7 |
| 2 | ordcmp/bytes | 2.433 | 125.6 | 51.5 |
| 3 | strconv/itoa | 1.496 | 135.7 | 91.1 |
| 4 | aesmodes/block | 1.406 | 8.7 | 6.3 |
| 5 | llvm/sieve | 1.317 | 219.6 | 165.4 |
| 6 | trimchars/trim_end | 1.294 | 18.6 | 14.3 |
| 7 | trimchars/trim_start | 1.291 | 19.0 | 14.9 |
| 8 | csv/format | 1.286 | 41.1 | 32.5 |
| 9 | image/over | 1.231 | 21.4 | 17.3 |
| 10 | strsplit/split_absent | 1.226 | 5.7 | 4.7 |
| 11 | netip/parse4 | 1.223 | 6.8 | 5.5 |
| 12 | cron/every5 | 1.217 | 122.7 | 101.2 |

## The 2026-11-07 top gaps now

| Case | 2026-11-07 | 2026-11-21 |
|---|---|---|
| metrics/counter | 4.251 | 0.740 |
| cbor/umarshal | 3.493 | 0.658 |
| time/format | 3.021 | 0.875 |
| time/parse | 2.824 | 0.576 |
| metrics/histogram | 2.718 | 0.799 |
| time/parse_layout | 2.006 | 0.517 |
| log/filtered | 1.793 | 0.076 |
| timego/parse | 1.776 | 0.959 |
| jsontoken/write | 1.732 | 1.135 |
| time/format_layout | 1.654 | 0.706 |

metrics/scrape, an output mismatch on 2026-11-07, now matches: 0.841.

## Not timed

- grpcreflect on host 1: the module pass stopped with no CPU for 15
  minutes in grpcreflect, and a rerun under `timeout 300` ended at the
  timeout (rc 124). On the Mac (darwin/arm64, xo built from the tree)
  the same twincheck runs: none 0.695, stream 0.565, files 0.357
  (`-time 1`, a diagnostic, not a host 1 row). Not investigated here.
- sqlmysql, sqlpg: need a database server and have no `twin.txt`; not
  twincheck cases by design (their own results:
  std-sql-mysql-2026-11-17.md, std-sql-pg-2026-11-04.md).
- sizewaste: output mismatch (first line differs), as on 2026-11-07.
- jitter: the Xo program was killed (`signal: killed`).
- http: a server, compared by parity.sh and measured by run.sh
  (release-vs-go's http section).
- 53 directories have no Go twin (`no twin.go.txt and no go/ to run`).
