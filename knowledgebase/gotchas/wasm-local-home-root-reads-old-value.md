# A local home's next root read the old value from a zeroed block

Status: Observed (2026-11-11, Xo Lang 2, wasm local homes)

## Symptom

With local homes (compiler/wasm home.xo add_local_homes), the playground
compiler built from the same source reported "concurrency: built again
with unwinding checks" for 29 of 135 programs that the compiler built
without them took in one pass.

## Cause

ssa.build_program_fe runs `b = program_builder(.., b.need_conc)`: a second
root of b's home whose argument reads the old b. The root call zeroed the
home's block (as a fresh sret block is) before its arguments were pushed,
so `b.need_conc` read false.

## Fix

A local home's block is zeroed only when it is made (the first root run);
a later root reuses it as it is, so its arguments read the old value and
the callee overwrites it. A root is also not a root when an argument it
passes by address (a home of the callee) is a field of a value of the
home's type: the callee would read it while writing its result there.

## How to avoid

A change to the wasm homes is checked by building the playground compiler
from the same source with xo before and after it and comparing what the
two build, program by program (bench/results/xoc-size-2026-11-11.md, local
homes): a miscompile of the compiler shows as a difference there before
any golden fails.
