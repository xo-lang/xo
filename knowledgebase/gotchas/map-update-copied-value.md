# Map.update copied the value it was meant to change in place

Status: Observed 2026-10-23, fixed (compiler/ssa map.xo map_update_take)

Symptom: a loop pushing into a map's list through `m.update(k, [],
fn(var l) { l.push(x) })` ran in time and allocations quadratic in the
list's length on the native backends: the checker's cand_events
allocated 98 GB for one 32,768 statement function (the scale suite).
The Go backend was fine.

Cause: the SSA lowering read the value (`list.index` of the map's
values list) and called the closure with it while the map still held
it. The value's count was two, so the closure's `var` parameter copied
it on its first write: one full copy per update, exactly the
copy-modify-store that decision 0266 tells people to replace with
`update`.

Fix: for a closure that cannot fail, the map moves out of its place and
the value out of the map with `list.take` (decision 0169), then back
with `list.set`. A closure that can fail still reads (the map keeps the
old value when it fails), as do a missing key and a log mode map
(decision 0291), which take the put path. A capturing closure still
allocates its environment at each call, which an allocation count of a
loop over update also shows. Test internal/llvm TestMapUpdateInPlace.
