# 0697: closures made and called once become direct calls (devirt_once)

Status: Accepted (xo-lang-1, 2026-11-17)

## Context

Task #211: std/trace Ring push ran 1.21 of Go on host 1. Its body is
`r.buf.with(fn(var st) { st.spans.push(s) })`: the closure captures the
Span s, so push allocated an environment, retained each of s's heap
fields when the closure read s back out of it, and released them again
when the environment was dropped; the call went through a function
value, which the inliner does not follow. Mutex.with, Map.update and
similar methods take their body as a closure literal, so this shape is
common in std (every locked update in std/sql, std/ui, std/trace).

## Decision

A new SSA pass, devirt_once (compiler/ssa/devirtonce.xo), runs after
specialize_args and before the reference counting on every native
backend (not wasm). For each Closure value with captures whose only use
is as the function of one CallValue in the same function, it makes a
copy of the closure's function (`NAME#once`) in which the environment
parameter is gone and each capture is an owned parameter, and rewrites
the call as a direct call of the copy with the call's arguments and then
the captures. The closure value goes away. The reference counting then
sees each capture move into the call (no retain on read, no release of
an environment), and the inliner may inline the copy.

A closure function qualifies when its environment is read only by
Capture, no capture is borrowed, it is not an entry or an export, and
it has no skipped parameters. A closure that catches (Mutex.with's
body, Map.update's with keep) keeps `catches` on its copy, and raises()
counts a direct call of a catching function as not raising, as it
already did for a CallValue of one. At most 512 copies a module.
XO_PASSES=-devirt_once turns it off.

## Consequences

Measured (host 1, `xo bench --ab`, 3 rounds, 2026-11-17): bench/perf/trace
wall 20.74 to 14.07 ms (B/A 0.678, 95% interval 0.65 to 0.689, better);
xo __text 9,986,236 to 9,725,540 bytes (-2.6%) on the LLVM column.

- std/trace Ring push: no environment and no retains; `record 2000000`
  on the Mac 0.12 to 0.13 s against 0.17 to 0.18 s (host 1 `--ab` in
  the change's report).
- A closure function used once gets a second copy; the original stays
  for its other users. Code size: see the change's __text report.
- test/codegen/onceclosure guards the shape: a Mutex.with body that
  captures a value makes no indirect call.

## Alternatives

- Retain elision across the environment (load-time borrow of captures
  while the environment is alive): keeps the allocation and the
  indirect call, and needs proof the environment outlives each read.
- Inline every closure literal at its one call site: the inliner works
  on direct calls; teaching it CallValue of a known Closure is the same
  rewrite with less reuse (the copy can still be called, not inlined,
  when it is large).
