# What `xo check --support` costs, and what the front end cache would save (2026-10-23)

Intel field report b08 option C (Xo Lang 2's proposal, lead queue): run
the native support check (XO1201) in every `xo check`, as a build does,
with its SSA step through the front end cache. Measured first, as the
option said.

## Method

- xo built by `go run ./scripts/xobuild` from main at the commit of this
  file's parent, on the development Mac (Apple M4 Pro, 24 GB), as a
  `HEAVY_CLASS=gate` job. Load average 17.6 at the start and 14.1 at
  the end (other agents' jobs): wall times are rough, read the
  differences, not the decimals.
- Programs: bench/perf/http/xo (a small HTTP server) and compiler/xo
  (the toolchain, about 1,800 .xo files with std).
- `xo check .` against `xo check --support .`, each run once untimed,
  then 3 interleaved timed reps; peak memory from `/usr/bin/time -l`.
- The front end cache's effect: `xo build` twice, the second with
  `--phase-times` (warm caches).

## Results

| program | `xo check` (3 reps) | `xo check --support` (3 reps) | peak, check / --support |
|---|---|---|---|
| bench/perf/http/xo | 0.57, 0.60, 0.60 s | 0.93, 0.94, 1.23 s | 0.91 / 1.00 GB |
| compiler/xo | 2.05, 2.02, 1.85 s | 3.60, 3.48, 3.34 s | 2.66 / 3.54 GB |

Warm `xo build --phase-times`:

| program | phase check | cache fe | phase ssa |
|---|---|---|---|
| bench/perf/http/xo | 73 ms | 2623/2637 bodies | 166 ms |
| compiler/xo | 650 ms | 9716/9894 bodies | 1082 ms |

## Reading

- `--support` adds about 0.35 s to the small server's check and about
  1.5 s to compiler/xo's, and 0.1 to 0.9 GB of peak memory: it builds
  the whole program's SSA without the front end cache
  (compiler/cli/core check.xo load_check, `ssa.build_program`).
- Through the front end cache (option C) the SSA step of a warm run is
  what a warm build pays: about 0.17 s and 1.1 s. So C would make a
  default support check cost about +0.2 s on the small program and
  +1.1 s on compiler/xo, instead of +0.35 s and +1.5 s.
- The larger gap is elsewhere: a plain `xo check` of the small server
  takes about 0.6 s while a warm build's check phase takes 73 ms (the
  module check cache, XO_CHK_CACHE=auto, decision 0351); compiler/xo
  2.0 s against 650 ms. `xo check` does not use that cache.
