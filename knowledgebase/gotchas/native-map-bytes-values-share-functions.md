# Map[K, Bytes] and Map[K, Str] share their native helper functions

Status: Observed (2026-10-12, decision 0178 stage 5: xo1 failed to build
once selfhost/cli/servers removed from a `Map[Str, Bytes]`).

## Symptom

`xo build --backend=arm64` of a program that calls `remove` on both a
`Map[Str, Str]` and a `Map[Str, Bytes]` stops with `ssa: ...: call
xo.map.remove[Map[Str, Str]] ...: result type does not match the callee`.

## Cause

Natively `Bytes` lowers to `Str`, so both maps are the SSA type
`Map[Str, Str]` and the generated helpers are named after it
(`xo.map.<op>[<map type>]`). Most helpers are then the same function,
which is right; `remove` also returns the removed value as an option, and
`Bytes?` and `Str?` are different SSA enums, so the second program point
called the first's function with the wrong result type.

## Fix

internal/ssa mapRemove and selfhost/ssa map_remove name a second remove
of the same map type with a different result `xo.map.remove[T][V?]`.
Golden internal/llvm/testdata/golden/map_remove_bytes. Any new map
helper whose signature mentions the value's option or another type built
from V needs the same care.
