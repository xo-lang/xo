# A --race build on the LLVM backend reports the runtime's own accesses as the program's races

Status: Observed (2026-11-05, xo-lang-4, the race stress).

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
