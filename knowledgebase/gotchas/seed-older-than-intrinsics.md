# An xo older than an intrinsic cannot build the selfhost/xo that uses it

Status: Observed (2026-10-13, decision 0215).

## Symptom

After merging main (syscalls2, decision 0206 stage 2) into the branch
that deleted the Go compiler, `go run ./scripts/xobuild` failed with
every seed older than that merge: the link of selfhost/xo stopped at
`Undefined symbols for architecture arm64: _xo_call_c2, _xo_sys_clone,
_xo_sys_words`. The go-compiler-final tag's xo1 (87873aa5) is older too.

## Cause

The merge added intrinsics (declared in std/native, lowered by
selfhost/ssa and the emitters) and used them in std/native code that
xo itself is built from. An older xo does not know them: it compiles
the calls as calls of external functions, which no C file defines.
Before decision 0215 the Go driver had the same intrinsics (every change
landed on both sides), so a Go built xo1 always knew them.

## Fix

Two hops (decision 0216 item 2): the older xo builds an intermediate
xo, the new compiler (main 4f792c55's selfhost/xo) against an older std
that does not use the intrinsics (the go-compiler-final std), and the
intermediate, whose emitters lower them, builds today's sources (311 s
each on the Mac). scripts/seed-chain.txt records the hop; internal/xotool
takes it when a seed fails, and scripts/bootstrap-xo.sh takes it with
XO_SEED_CHAIN=file. A change that makes xo's own sources use something
an older xo cannot compile records its hop in the same change.
