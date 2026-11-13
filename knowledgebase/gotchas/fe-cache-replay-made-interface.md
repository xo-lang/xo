# An interface type a replayed cache entry made had no methods for the builder

Status: Observed (2026-11-13), fixed

Symptom: `xo build` of a program that logs (internal/llvm/testdata/golden
log_error) failed with XO1201 at std/native/log.xo:163 and :180, "a call
of the interface method `Handler.enabled` with a missing or `var`
argument", after another program's build had filled the front end cache.
The warning before it named the entry for log.record: "its body did not
decode: no fn(log.Level) -> Bool, fn(log.Record) -> () here ...
building again without it", and the rebuild failed the same way.
XO_FE_CACHE=off built it. The 2-hourly windows/amd64 cross build hit it
first, depending on which sampled golden built first; on an empty cache,
log_state (or log_lazy, log_with) and then log_error repeats it on any
native target. 0.51.0 has it (its release notes list it as a known issue).

Cause: a front end cache entry makes the named types it needs before its
bodies decode (fecache.xo), `xo.iface[I]` among them, with its shape.
compiler/ssa/iface.xo `iface_type` returned at once for a named type that
already existed, so the builder's side tables for it were never made: its
methods (`ifaces`) and, in decision 0613's itab form, each slot's function
type (`itab_ft`). A body then built from source read each slot's type as
its Int code word, found no parameters, and reported XO1201; the entry's
own decode missed the slot function types the same way. Never silent: a
method call needs `ifaces` (method_on), and equality or a `var self` call
through such a type reaches a CallValue on an Int, which the driver's
verifier rejects.

Fix: `iface_type` returns early only when the builder has lowered the
type (`ifaces` has it); otherwise it makes the side tables and sets the
same fields again. internal/arm64 TestFrontCacheReplayMadeInterface builds
log_state then log_error on an empty cache, natively and for
windows/amd64.

Avoid: a builder table keyed by a type the module can already hold (made
by a replay, by another builder) must not be filled only when the type is
created; check the table, not `created`.
