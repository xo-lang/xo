# A `var` closure that keeps its given value copies a map on every write

Status: Observed (2026-11-03, Xo Lang 2)

## Symptom

std/cookiejar ran at 1.5 to 1.7 of Go's net/http/cookiejar, and XO_PROF
showed `xo.maph.slowput` and `xo.mlog.*` frames for a map of maps
(`Map[Str, Map[Str, Entry]]`) although nothing visibly shared the inner
map. A 20 line repro (an inner map written through `m.update(k, {}, fn(var
sub) { sub[id] = ... })` on a local) took 43 profile samples where the
same writes to a flat map took 10, all in the map log's put.

## Cause

`Map.update`'s closure was built as Mutex.with's is: it caught a fault
and handed back the value it was given (Func.catches, `pack_orig`), so
the value had to stay alive across the closure's body. The rc pass then
retained each of the inner map's lists before the put, the put saw them
shared and copied them, and decision 0291's hybrid map turned the value
into its log form after the second such copy, for good. Nothing read the
kept value: update's caller unwinds on a fault and stores only on
success.

## Fix

`with_closure(..., keep: false)` for Map.update (compiler/ssa/map.xo):
its closure neither catches nor keeps the value. Mutex.with still does,
because core 5.4 promises the protected value survives a fault, so a map
inside a Mutex's value is still copied once per `with` that writes it
and then runs in the log form (its O(1) copies are the reason it exists).
test/codegen/maps checks that the closure makes no run of three retain
calls (seven before the fix).

To spot it: `XO_DUMP_FN=NAME.fn1` and look for `retain` of a `var`
parameter's fields before a call that consumes it; in `xo asm`, a run of
`bl _xo_h..._xN` retain thunks before `xo.map.put`.
