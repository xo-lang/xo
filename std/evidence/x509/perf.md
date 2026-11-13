kind: perf
module: x509
result: fail
commit: 16beb59d9
source: 7372ce42930af6d0aae0471de38e283308d8ce5de66c7e32c76d7a703dd46047
date: 2026-11-13
host: linux/arm64 host 1
command: go run ./scripts/stdproven perf -rounds 9 -host "linux/arm64 host 1" x509
measure: 7 cases of 1 twin directories, worst Xo / Go 1.998 over the cases above 5 ms (Proven at or under 1.05), 1m2s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 9 over bench/perf/x509; medians of wall time, the two programs alternating.

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/x509/none (under 5 ms: startup) | 0.8 | 0.5 | 0.599 |
| bench/perf/x509/parse | 445.1 | 415.7 | 0.934 |
| bench/perf/x509/verify | 666.0 | 507.0 | 0.761 |
| bench/perf/x509/verify150 | 281.5 | 562.5 | 1.998 |
| bench/perf/x509/pool150 | 282.7 | 226.9 | 0.803 |
| bench/perf/x509/create | 1064.0 | 856.1 | 0.805 |
| bench/perf/x509/crl | 460.3 | 155.2 | 0.337 |

verify150 verifies against a List of 150 roots, which the native backend
parses on every call; Go's twin always passes a CertPool (Go's
VerifyOptions take only pools), so the row compares two different
APIs. The same verify through an x509.CertPool (decision 0626,
16beb59d9) is pool150: 0.803 of Go. Next: parse a list's roots
lazily, by subject, as Go's lazyCert does, so verify150 needs no pool.
