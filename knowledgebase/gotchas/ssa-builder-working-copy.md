# The SSA builder's function is a working copy until finish_fn

Status: Observed (2026-11-16)

## Symptom

A field set on a function while its body is built (compiler/ssa
declare_c_export: `b.m.funcs[w].wasm_i32.push(i)`) is gone in the
finished module: the wasm reactor's Str parameters came out as i64
leaves, with no error.

## Cause

`b.begin(w, ...)` copies `b.m.funcs[w]` into the builder's working
function `b.s.f`, which the body's instructions go into, and
`b.finish_fn()` writes `b.s.f` back over `b.m.funcs[w]`. A write to
`b.m.funcs[w]` between the two is lost.

## Fix

Between begin and finish_fn, set fields on `b.s.f` (the wasm_i32 marks
do). A new Func field must also be carried by every Func copy
(compiler/ssa ir.xo, build.xo and rccache.xo build Func values field by
field).
