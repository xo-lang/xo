# /alloc at two .data shifts: the 128 byte false sharing check (decision 0459), 2026-11-02

Question: did 0.36's /alloc gap on the first linux/arm64 host (a Linux VM on an
M4 Pro, whose cores have 128 byte cache lines) come from false sharing at 128 bytes in
rt.static_word's table (xo_static), whose phase against the lines moved with .text and .data
size? Method: every server built on main a144a3115 with each variant's xo, then linked twice
with XO_DATA_SHIFT=0 and 64 (Xo's ELF linker puts that many zero bytes before .data's first
input section; the code is byte for byte the same), and measured in one job.

- Host: the first linux/arm64 offload host, exclusive (OFF_EXCLUSIVE=1), 12 CPUs, load 7.0
  at the start; servers pinned to CPUs 0-5, the loadgen to 6-11.
- Load: bench/perf/jitter's loadgen, closed loop, `-path /alloc -c 256` and `-c 32`, `-d 10s
  -warmup 2s -awake`; 3 rounds, the servers' order rotated each round.
- Scripts: alloc-shift.sh and alloc-shift-remote.sh (from Xo Lang 4's alloc-points runner);
  raw output: alloc-shift-2026-11-02/alloc-shift.txt (each run's JSON line and its load).
- Variants (the experiment revisions are unpushed branches; only the first row's knob and
  both tables' alignment landed):
  - main + XO_DATA_SHIFT only: a144a3115 with the linker knob (629e9cef9);
  - xo_static aligned to 128: Xo Lang 1's one line change on it (0992a036f);
  - both tables aligned and heapos' and poll's written words on 128 byte lines of their
    own (8f2ee5a1c, branch l2-pad);
  - that with Xo Lang 4's Response without its ws field (3ff5f4fd3, from ec2398ed8);
  - that with the pool record's hot words 128 bytes apart (4e3588906, branch l2-poolpad0).

Where the tables landed (nm; each pair: shift 0, shift 64):

| variant | xo_sys_words | xo_static |
|---|---|---|
| main + knob | 0x4fc308, 0x4fc348 | 0x4fc388, 0x4fc3c8 (phase 8, then 72 of 128) |
| xo_static aligned | 0x4fc3a0, 0x4fc420 | 0x4fc480, 0x4fc500 |
| both aligned + own lines | 0x4fc400, 0x4fc480 | 0x4fc480, 0x4fc500 |
| + no ws | 0x4fc180, 0x4fc200 | 0x4fc200, 0x4fc280 |
| + pool split | 0x4fc400, 0x4fc480 | 0x4fc480, 0x4fc500 |

Medians of 3 rounds; CPU µs a request with its range and its ratio to main + knob at shift 0;
req/s with its range:

| server | shift | c256 CPU µs/req (ratio) | c256 req/s (range) | c32 CPU µs/req (ratio) | c32 req/s (range) |
|---|---|---|---|---|---|
| main a144a3115 + XO_DATA_SHIFT only (629e9cef9) | 0 | 44.7 (44.4-44.9; 1.000) | 134.3k (133.6-134.9) | 44.9 (44.8-45.2; 1.000) | 133.0k (132.2-133.4) |
| main a144a3115 + XO_DATA_SHIFT only (629e9cef9) | 64 | 44.9 (44.6-45.9; 1.006) | 133.5k (119.9-134.5) | 45.4 (44.9-45.5; 1.011) | 131.5k (131.2-133.0) |
| xo_static aligned to 128 (Lang 1's change, 0992a036f) | 0 | 44.4 (44.3-44.6; 0.995) | 134.9k (134.4-135.2) | 44.7 (44.6-45.2; 0.995) | 133.7k (132.3-133.9) |
| xo_static aligned to 128 (Lang 1's change, 0992a036f) | 64 | 44.5 (44.5-44.7; 0.997) | 134.7k (134.2-134.8) | 45.1 (44.8-45.1; 1.004) | 132.5k (132.3-133.1) |
| both tables aligned + written words on own lines (8f2ee5a1c) | 0 | 44.6 (44.6-44.7; 1.000) | 134.3k (134.1-134.3) | 45.0 (44.8-45.2; 1.002) | 132.8k (132.1-133.4) |
| both tables aligned + written words on own lines (8f2ee5a1c) | 64 | 44.5 (44.4-44.6; 0.996) | 134.8k (134.4-134.9) | 45.2 (45.2-45.2; 1.006) | 132.2k (132.2-132.3) |
| 8f2ee5a1c + Response without ws (3ff5f4fd3) | 0 | 40.4 (40.3-40.8; 0.906) | 148.2k (146.8-148.9) | 40.8 (40.3-41.2; 0.908) | 146.0k (144.3-147.7) |
| 8f2ee5a1c + Response without ws (3ff5f4fd3) | 64 | 40.5 (40.3-40.6; 0.907) | 148.0k (147.8-148.7) | 40.8 (40.3-41.4; 0.908) | 146.1k (144.0-147.5) |
| 8f2ee5a1c + pool record split (4e3588906) | 0 | 44.3 (44.2-44.5; 0.992) | 135.4k (134.7-135.5) | 44.9 (44.9-45.2; 1.000) | 133.0k (132.3-133.1) |
| 8f2ee5a1c + pool record split (4e3588906) | 64 | 44.6 (44.4-44.8; 0.998) | 134.5k (133.7-135.1) | 45.2 (45.2-45.3; 1.008) | 132.0k (131.8-132.1) |

Reading:

- Moving main's .data by 64 bytes (xo_static to another 128 byte phase, code unchanged)
  moved CPU a request by 0.6% (c256) and 1.1% (c32); one c256 round at 119.9k req/s is
  the only outlier. The hypothesis is refuted: the 25% gap between linger and linger plus
  narrow B does not come from .data placement.
- Aligning the tables, giving the written words lines of their own and splitting the pool
  record each move CPU a request by less than 1%, at either shift.
- Response without its ws field: 0.906 to 0.908 at both shifts and both concurrencies, about
  10% more req/s; layout independent.
- Landed (decision 0459): both tables 128 byte aligned and XO_DATA_SHIFT. Code placement is
  checked next with decision 0075's XO_ARM64_PAD (Xo Lang 1).
