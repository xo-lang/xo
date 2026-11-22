kind: perf
module: pty
result: fail
commit: 176567e09
source: 28f91726c9733a75c834215e7f44bf0a1debbe8bf4781241af0ac922752240a8
date: 2026-11-22
host: linux/arm64 host 2
command: go run ./scripts/stdproven perf -rounds 30 -host "linux/arm64 host 2" pty
measure: 5 cases of 1 twin directories, worst Xo / Go 1.449 over the cases above 5 ms (Proven at or under 1.05), 1m18s

Performance against Go (decisions 0461, 0616): scripts/twincheck -time 30 over bench/perf/pty, the two programs alternating; Go ms and Xo ms are the medians of wall time, Xo / Go the median of the per-round ratios (decision 0726; a case under 10 ms of Go runs until each side has run 1 s).

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/pty/none (under 5 ms: startup) | 0.4 | 0.4 | 0.863 |
| bench/perf/pty/open | 35.9 | 27.8 | 0.777 |
| bench/perf/pty/size | 110.6 | 49.4 | 0.449 |
| bench/perf/pty/echo | 815.1 | 1169.8 | 1.449 |
| bench/perf/pty/spawn | 60.0 | 65.5 | 1.096 |
