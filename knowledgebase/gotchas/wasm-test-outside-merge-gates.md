# TestSelfhostWasm was red for three releases unnoticed

Status: Observed (2026-10-16, found by Xo lang 4; xo-4b fixed it).

## Symptom

TestSelfhostWasm failed on main with invalid modules (`call[4] expected
type i32, found local.get of type i64` in tls13srv.pump) and, once
those validated, run time faults. Seeds 0.15.0, 0.16.0 and 0.17.0 all
had it red.

## Cause

The test runs only in internal/selfhost's merge and full tiers, and
only when Node is installed. The per-merge gate after the arm64-only
rule (CLAUDE.md: -short, the touched packages' tests, the fixed point)
runs neither for a change under std/native/llrt, where the TLS and
crypto code that broke the wasm build lives; the fixed point never
builds selfhost/wasm. So std changes that the native backends compile
fine went in without a wasm run.

## Fix

The wasm causes (2026-10-16): selfhost/wasm release_run batched a
struct's field releases from locals that alloc_leaves had trimmed (a
var parameter's write back); rt.heap_large and its resize were missing
in the wasm runtime; stdin.isatty took the host's arm in a WASI module.
The process: the lead runs TestSelfhostWasm before every seed cut. A
change under std/native/ or selfhost/wasm/ should also run it (`go
test -run TestSelfhostWasm ./internal/selfhost`, about 6 minutes).
