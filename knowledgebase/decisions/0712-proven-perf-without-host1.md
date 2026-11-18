# 0712: Proven performance evidence is not taken on host 1

Status: Accepted (owner, 2026-11-17). Amends 0616 and 0708 (where perf
evidence is timed); written by the lead.

## Context

Performance evidence for std Proven (0616's perf rows, `stdproven perf`,
`stdproven serverperf`) was timed on host 1 (linux/arm64, `offload.env`, an
M4 Pro). On 2026-11-17 five modules were slower than Go only there: metrics
counter 1.27 to 1.34, netip parse 1.20, toml decode 1.19, blake2b keyed
1.18 and log json 1.16, while the Mac (M5) measured them at or under Go
(netip 0.73, blake2b 0.51, log json 0.99). QEMU instruction counts (owner
run, 2026-11-17) show netip running fewer instructions than Go yet taking
1.5x on host 1: a per-chip difference in instructions per cycle that only
the M4's hardware counters can explain. The owner (2026-11-17): take host
1 out of the Proven route and leave it for a task after Proven.

## Decision

- New perf evidence is timed on the linux/amd64 host (0708: a lane
  exclusive lease, Go's twin on the same host in the same job) or on an
  arm64 pass/fail host (2, 3 or 4) under an exclusive lease. Not on host 1.
- A module's perf row from host 1 that already meets the bar (worst Xo/Go
  at most 1.05) stays valid. A row from host 1 that misses the bar is
  retaken on amd64 (or an exclusive arm64 host 2 to 4); the newer row
  decides, as 0708 says.
- Thresholds are unchanged (0616, 0461: at most 1.05 of Go).
- Why Xo runs fewer instructions per cycle than Go on host 1 is a
  performance task after Proven (xctrace on the M4, Lang 2's
  ~/xo-lead/notes/lang2/xctrace-m4/), not a Proven blocker.
- Host 1 stays the host for `xo bench --ab` speed claims (decision 0340);
  this changes only where Proven perf evidence is taken.

## Consequences

- stdproven perf and serverperf default to the amd64 lane; stdcov accepts
  their rows (xw-1, task #297).
- A module can reach Proven on performance while its host 1 gap is open;
  Proven says Xo is within 1.05 of Go on the measured host, not on every
  chip.

## Alternatives

- Keep host 1 and explain the gap first (xctrace): blocks five modules on a
  question of one chip.
- Time on the Mac: the Mac runs gates and agent work, so its timings are
  noisy; the offload hosts under an exclusive lease are quieter.
