# 0317: Inlining a closure with a var parameter (catches) at a call with a landing

Status: Accepted (the lead, 2026-10-21; Xo lang 4, reviewed by Xo Lang 2)

## Context

compare-2026-10-21.md ranked bench/perf/xo update first among the
native backends' gaps to LLVM (amd64 1.32, arm64 1.26). Per function
cachegrind on linux/arm64: the closure given to Map.update,
`m.update(k, 0, fn(var v) { v += 1 })`, runs out of line
(update_loop.fn1, 27 M instructions), where LLVM inlines it.

The arm64 inliner refused it. Its new reason in XO_INLINE_REPORT
(`no (hard: catches)`, added here): ssa map.xo closure_with makes every
closure with a `var` parameter `Func.catches`. A fault in it hands back
the value it was given (its unwind exit returns the parameter, without
rt.fault_continue), and the caller goes on unwinding from the call's
landing, where it runs rt.fault_continue and has not yet written the
new value (it writes it only after a normal return). The inliner
refused every catches function, and devirtualize did not make a call of
one direct. The `var` parameter itself is no frame home: SSA returns it
in a tuple (fn(Int) -> ((), Int)).

## Decision

A catches closure is inlined where its call lands (x.pad >= 0), when
its parameters (but the closure) hold no heap memory (an owned var
parameter, a Map updated in place, is handed back by the exit, so in the
copy it would still be held on the way to the call's landing; the
verifier refused nested.update's fn(var inner) in TestGoldenRun/maps),
every landing of the closure only reaches its unwind exit through
blocks that jump and nothing else, and the exit only makes the zero
results and the handed back value and returns (Const, Struct, Tuple,
Variant, Undef, Field, Return, an rt.*_empty call): then no cleanup is
on the way or there, and in the copy each of the closure's landings is
the call's landing, so a fault in the copy reaches the same code with
the same state the call would have. compiler/arm64 inline.xo:
catch_inlinable and hands_back; find_call, find_closure_call and
find_loop_call check it for a catches callee; devirtualize makes a
catches closure's call direct where it lands; inline_call sets the
copy's landings to the call's for a catches callee.

## Consequences

- bench/perf/xo update (Mac arm64, instructions retired, best of 3):
  425.1 to 382.9 million (-9.9%) against main's xo.
- New golden map_update_fault: lambdas that change their parameter,
  then fault in a nested call, one owning a string it built (Xo Lang 2's
  review): the same output on arm64, LLVM and the Go backend, no leak
  (XO_LEAKCHECK), under XO_INLINE_STRICT=1.
- A closure with a heap parameter, or whose landing or exit releases
  something, stays a call (TestGoldenRun/maps failed under
  XO_INLINE_STRICT=1 until the heap rule: the edge into the call's
  landing held the handed back Map). Counters, sums and flags (Int,
  Float, Bool values) are the case inlined.
- xo itself (main's compiler/xo and std, built by main's xo and by
  this change's, Mac): __text the same, arm64 42,375,336 and LLVM (split
  12, no LTO) 42,959,128 (xo has no Map.update closure with scalar
  parameters); the cold arm64 self-build (XO_FE_CACHE=off,
  XO_BACKEND_CACHE=off) 146.6 against 141.9 s of CPU, within noise. The
  fixed point holds (scripts/gate.sh fp).

## Alternatives

- A frame home for the var parameter in the caller: not the cause (the
  parameter is a returned tuple).
- Specializing std's higher order functions per closure literal: hof's
  lambdas are already inlined (decision 0256's padded rule); its gap is
  per iteration code, not calls.
- Rewriting the copy's landings to run the closure's cleanup and then
  the caller's: more general, more code on each fault path; not needed
  for update's shape.
