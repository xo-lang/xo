# An intrinsic mapped before it is lowered: the next xo calls a symbol that does not exist

Status: Observed (2026-10-21, decision 0329)

## Symptom

`go run ./scripts/xobuild` succeeded, and running the new bin/xo failed
at once: `dyld: Symbol not found: _xo_chacha_blocks4`.

## Cause

compiler/ssa had the mapping of cpu.chacha_blocks4 to rt.chacha_blocks4
before the backends had its lowering, and `go run ./scripts/xocheck`
built the tree's bin/xo at that moment (it builds when the tree has
none). xobuild then built the finished tree with that intermediate xo:
its SSA made the intrinsic, its backends did not know it, and an
unknown rt.NAME becomes a call of the C symbol xo_NAME. xo's own code
reaches the intrinsic (it includes std's TLS), so the new compiler
itself had the dangling call.

## Fix

Add the backend lowerings before (or with) the SSA mapping, or delete the
intermediate bin/xo before the build (and its shared cache entry,
`<user cache>/xo/checkout/<key>`, by its literal path) so xobuild starts
from an ancestor's xo, which maps nothing. Seeds and main's xos never
map without lowering, so only a work tree's intermediate build hits it.
Related: gotchas/seed-older-than-intrinsics.md.
