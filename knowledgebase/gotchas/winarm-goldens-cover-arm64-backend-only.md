# The winarm golden run covers the arm64 backend only

Observed 2026-11-21 (task #605, candidate 9 at 347d11e0b).

## Symptom

On the windows/arm64 VM, scripts/winarm-goldens.sh passed 848 of 852
goldens, but 33 cmd/xo tests failed to link a program built with the LLVM
backend: `lld-link: error: undefined symbol: xo_async_stub`, referenced
from `x.pool.mon_scan` (std/native/llrt/pool.xo win_kick).

## Cause

winarm-goldens.sh builds every golden with `--backend=arm64`, so nothing
built a windows/arm64 program with LLVM before the cmd tests did (xo on a
Windows host with clang picks LLVM for `xo test` and several cmd tests).
pool.xo win_kick names `xo_async_stub` on every Windows target; the arm64
emitter defines it on windows/arm64 as a trap (compiler/arm64 sched.xo),
but compiler/llvm sched.xo preempt_asm defined it on windows/amd64 only.

## Fix

compiler/llvm sched.xo async_stub_asm defines the stub on windows/arm64
too (`brk #1`, as the arm64 emitter's: windows/arm64 has no asynchronous
preemption yet, so it never runs). Test: compiler/llvm sched_test.xo "the
asynchronous preemption stub is defined on both Windows arches".

To catch the next one: the cmd/xo run on the VM (offload-winarm.env,
`go test ./cmd/xo` with XO_CLI set to a windows/arm64 xo.exe) is the only
check that builds windows/arm64 programs with LLVM.
