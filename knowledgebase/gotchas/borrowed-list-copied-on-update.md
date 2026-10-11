# Borrowing a list or string parameter that a path consumes makes every update copy

Status: Observed (2026-10-10)

## Symptom

The first version of decision 0149 (borrow a parameter unless every path
consumes it, for every type) made the self hosted checker's LLVM build
take 76 s for 20 runs instead of 3.1 s. The profile was 2,800 of 3,500
samples in `memmove` under `xo_rl_copy`, called from
`xo.map.put[Map[Int, Int]]` and `xo.map.put[Map[Int, CallInfo]]`.

## Cause

`put` consumes its map on every path but one, so it became borrowed.
Its callers keep the map, so `put` retains it (count 2) where it
consumes it, and the in place list updates inside (`list.set`,
`list.push` through `reuse`) see a shared list and copy it: every insert
copied the map's arrays, quadratic in the map's size.

## Fix

Only enum parameters follow the new rule (rc.go `inferBorrows`): enums
are never updated in place and their payloads are retained when consumed
anyway. Before widening borrow inference to a type, check whether a
value of that type is ever updated in place when unique (lists, strings,
and anything holding them that is rebuilt with `setfield` and then
updated); benchmark with a map heavy program, not only a tree walk.
