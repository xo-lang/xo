# A new one-file std module left out of std/merged.txt crashes stdgen

Status: Observed 2026-11-15 (Xo Lang 3, std/brotli).

## Symptom

`go run ./scripts/stdgen -check` (a gate step for every std change) fails
with a fault inside the compiler, not a message:

```
stdgen: xo dev dump go --std-types: exit status 70
fault: index 0 out of range for List of length 0 at gen/expr.xo:501:12
  github.com/xo-lang/xo/compiler/gen.Gen.coll_lit (expr.xo:501)
  github.com/xo-lang/xo/compiler/gen.Gen.gen_std_const (stdtypes.xo:161)
```

`xo test --std-module=M` passes, so the module itself is fine.

## Cause

A std module with its implementation in `std/M.xo` (no std/native/M.xo)
is a merged module (decision 0588) and must be listed in std/merged.txt.
Unlisted, gen's std_types treats it as a declarations-only module and
generates each constant for std/rt, and a `List` constant's literal there
has no element type (`targs` is empty), so coll_lit faults. A `Bytes` or
`Int` constant does not fault: it only shows the next problem,
`std/rt/std_gen_M.go is missing: run go run ./scripts/stdsync`.

When bisecting, look for the fault line: every one-constant file fails
`-check` (the missing std_gen file), so a pass/fail bisect points at
every constant at once.

## Fix

Add `std/M` to std/merged.txt (and `M` to std/go_compiled.txt once the
module has a scripts/stdcorpus corpus, as jpeg, otp and publicsuffix
do). The compiler's fault is a robustness bug to fix separately: an
unlisted module should get a message naming merged.txt.
