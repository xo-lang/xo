# Decision 0465 step 2 landed: functions that return a view (2026-11-03)

Xo Lang 1. `xo bench --ab` on the first linux/arm64 offload host (ab-host.sh
and the self build scripts in ~/xo-lead/notes/lang1). A = main at
1e5f98fe6, B = step 2 rebased onto it, except where a row names another A.

## Generated code

5 rounds, A = 1e5f98fe6, B = e04e409da (the step 2 squash):

| program | wall B/A | 95% interval |
|---|---|---|
| bench/perf/trimchars trim | 0.741 | 0.663 to 0.76 |
| bench/perf/trimchars trim_end | 0.728 | 0.697 to 0.744 |
| bench/perf/retview use | 0.721 | 0.72 to 0.734 |
| bench/perf/retview keep | 0.746 | 0.727 to 0.845 |

max RSS 1.0 on all four. The ceiling (retview-ceiling-2026-11-03.md) was use
0.702, keep 0.755.

bench/perf/jitter's HTTP server (scripts/release-vs-go-http.sh, base = an xo
built at main, XO_ARM64_PAD 0 to 3, 3 rounds; bench/perf/jitter/pads.py's
verdict, req/s of step 2 against main, the median over pads):

| row | req/s xo/base | verdict |
|---|---|---|
| /hello c32 | 1.001 | placement |
| /hello c256 | 1.000 | placement |
| /alloc c32 | 0.998 | placement |
| /alloc c256 | 1.007 | placement |
| /alloc c64 open 10k/s | 1.000 (CPU/req 52.00 against 51.75 us) | placement |
| /alloc c64 open 20k/s | 1.000 (CPU/req 57.37 against 57.37 us) | placement |

No change: the server's request path has no hot function that step 2
marks.

## xo's self build

A = 219d17223 (main when measured), B = a8b75ae1 (step 2 before the rebase;
the rebase brought in no compiler/ssa or compiler/arm64 inliner change).

- Both xo built by the seed (ab-host.sh self, 5 rounds): step 2's compile
  cost. wall 1.007 (0.997 to 1.010), cpu 1.003 (0.999 to 1.006), max RSS
  within noise.
- Each xo built by itself (s2, as scripts/bootstrap-xo.sh), each building
  B's compiler/xo with XO_INLINE_STRICT=1, 8 rounds after the host's load
  fell below 0.5: wall 1.004 (0.999 to 1.009), cpu 1.004 (0.998 to 1.008),
  max RSS 1.002 (0.994 to 1.018), all within noise. The load around 6 in
  the rounds is the build's own threads (--keep-loaded).
- Phase times of one xo (B) building compiler/xo with the pass on and off
  (XO_PASSES=-ret_views, 4 runs each): insert_rc 8,298 against 8,254 ms
  (+44), emit 8,821 against 9,126 ms (-305); the others within 15 ms once
  the deferred inline verification (off with any XO_PASSES) is counted in
  `inline`.

## Superseded readings

- Before Rc.rvn (a hash of the callee's name in kind() for every call
  value): self 1.011 (1.004 to 1.024) both seed-built and 1.010 (1.007 to
  1.012) self-built; insert_rc +122 ms.
- A draft at b7297e3f read 0.94 wall and 0.885 RSS: its verifier lacked
  Rc.rvn, the inlined module did not verify, and the build fell back to no
  inlining (gotchas/ab-self-measures-inline-fallback.md). ab-host-remote.sh
  now sets XO_INLINE_STRICT=1.
