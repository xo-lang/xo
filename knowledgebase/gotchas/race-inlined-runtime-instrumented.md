# A --race build on the LLVM backend reports the runtime's own accesses as the program's races

Status: Observed (2026-11-05, xo-lang-4, the race stress); observed
again the same day (the darwin rerun at e06836883, see "Second cause").

## Symptom

`--race` builds of http_h2_config (darwin, every run) and
websocket_tls_deadline (linux, 1 run in 24) stopped with ThreadSanitizer
data race reports whose accesses were all the runtime's:
- hconn.tls_ops_set's compare and swap of the TLS ops table word,
  against a plain read of that word inside `http.Server.with_protocols`;
- `pool.held` from poll.lk, against the same field from
  hserve.stream_put inside `http.sput`.
Decision 0273 mode 2 leaves the runtime (std/native/llrt) out of
instrumentation, so neither should have been reported.

## Cause

The LLVM backend left the runtime out per function only: an llrt
function gets the attribute set without `sanitize_thread` (func.xo
race_unchecked). xo's own inliner had already copied llrt code into
the program's functions (`srvh2c.install_h2c` into `with_protocols`),
so that code was instrumented under the caller's `sanitize_thread`.
The arm64 backend already skipped such instructions by their inline
site (race.xo race_instrument).

## Fix

The LLVM gen marks every load, store, atomic and memory intrinsic of a
value whose innermost inline site is an llrt file with `!nosanitize
!10`, which LLVM's ThreadSanitizer pass skips (func.xo emit,
race_access). internal/llvm TestRaceRuntimeInlinedUnchecked builds the
affected goldens with --race on the LLVM backend and fails on any
report. A new per-backend --race rule needs both places: the function
attribute and the inline sites.

## Second cause: LLVM's own inline of an alwaysinline call

The darwin rerun with that fix still reported the tls_ops_set case in
http_h2_config (1 run in 12; 13 in 24 on one build). The IR was right:
`with_protocols` called `srvh2c.install_h2c`, it was not xo's inline.
But the call site carried `#6` (`alwaysinline`, call.xo for a callee in
`g.inl`), and LLVM inlines an always-inline call over the two functions'
sanitize attributes (without it, it never inlines a function without
`sanitize_thread` into one with it). The copied code then sat under
`sanitize_thread`, and `!nosanitize` did not hold: at -O2 the optimizer
made tls_ops_set's conditional reload unconditional (a select) and
dropped its metadata, so TSan instrumented the read. A 25 line IR
reproduces it with Apple clang 21 (an alwaysinline call of a function
whose cmpxchg is followed by a `!nosanitize` load on its failure path:
`__tsan_read8` at -O2, none at -O0 and -O1).

Fix: under --race (mode 2) an instrumented function's call of a runtime
function is never marked alwaysinline (call.xo race_keeps_call). Test:
internal/llvm TestRaceRuntimeCallsNotForced (the non-race IR keeps the
mark, the race IR drops it); http_h2_config --race reported 0 of 24 on
darwin after.

Still open: code that xo's own inliner copies (the first cause) relies
on `!nosanitize`, which the same speculation can drop. If a report of
an llrt access comes back from a program function, the next step is to
stop xo's inliner from inlining llrt functions into program functions
under --race on the LLVM backend (compiler/ssa inline's candidate
filter, keyed on the build's race flag), as LLVM does by attribute.
