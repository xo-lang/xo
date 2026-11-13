# 0625: A fresh install's first edit reuses its program's caches

Status: Rejected (Xo Lang 2, 2026-11-13): the mechanism exists; the gap was the measurement's

## Context

release-vs-go v0.50.1's armcompute row (bench/results/release-vs-go-v0.50.1.md) showed a fresh
install's first edit at 471 ms against Go's 304, where an edit with std's variants already built took
226 ms: the cold build ran whole program, and the edit reused none of the separate build's caches
(fe 0/57 bodies, chk 0/9 modules, functions 0/450). The lead asked for a decision on a background fill
of those caches after a build, with a lock, nice, an opt-out, a memory cap, its behavior when the
next build starts, and measurements.

## Decision

No new mechanism. Decision 0388's background std build already does this: after building std's
variants, its child reruns the build that started it with the output thrown away
(XO_STD_BG_WARM_OUT, compiler/xo/stdcache.xo std_variant_peek, since 2026-11-05), which fills the
front end, module check, function and part caches. That child already has the properties asked for:
one at a time (`<cache>/std/.bg.lock`), detached and niced, started after the build ends (or after a
minute for a run that keeps going), off with XO_SEPCOMP_BG=0 and on windows hosts, and a build that
starts while it runs builds whole program and starts none.

The 471 ms was the measurement's: release-vs-go ran under heavy.sh, whose jobs set
XO_SEPCOMP_BG=0, and built the variants by hand with `xo dev stdcache`, which skips the warm step
(gotchas/heavy-runs-no-background-std-fill.md).

Measured with the background build on (Mac, xo 0.50.1, armcompute, empty caches; 2026-11-13):

| step | wall ms | peak MB | reused |
|---|---|---|---|
| cold (whole program; starts the background build) | 3714 | 544 | fe 0/3227, chk 0/9 |
| background build done | 29 s after cold | - | - |
| second (run cache hit) | 41 | 21 | - |
| first edit | 233 | 160 | fe 49/57, chk 8/9, functions 175/450 |
| second edit | 204 | 155 | fe 49/57, chk 8/9, functions 176/450 |

## Consequences

- A user who edits within about 30 s of a fresh install's first cold build of a program this size
  still gets a whole-program edit (the background build is running); after it, edits are about
  0.2 s, against Go's 0.3 s.
- release-vs-go's armcompute row reports the variants-first row as the comparison with Go and the
  fresh-install row's first edit as what the background fill replaces.
- Not measured here, open for a later change: the background build's own peak memory and CPU
  (it ran outside heavy.sh's accounting), and coll's sequence.

## Alternatives

- A second background fill keyed on a run-cache hit (the proposal this ADR was claimed for):
  redundant with the warm step.
- Make the cold build store separate-path entries too: more cold-build work for every program, for
  an edit that the background build already serves 30 s later.
