# The inliner's cycle check missed recursion through a closure literal

Observed 2026-11-06 (Xo Lang 1; bisected by Xo Lang 4).

## Symptom

After b29c306c8 (compiler/ssa passes.xo prune_unwind walk_literal), an xo
built from main ran out of its memory budget building compiler/xo:
"memory budget exceeded ... past phase inline.read". On linux/arm64 host 1,
xo@b29c306c8's inline.apply did not finish in 28.5 minutes and reached
22.5 GB, where its parent's took 5.8 s and the whole build 4.15 GB. Every
phase before inline.apply matched.

## Cause

compiler/arm64/inline.xo inline_walk keeps a function that calls one still
being walked out of the inlinable set ("so copies never nest without
end"), but it followed only calls. A function f that hands a closure
literal calling f to a prelude walk (`t.kids.map(fn(k) { f(k) })`) has no
call cycle: f calls list.map, and the literal is a value. Once the walk is
inlined, devirtualize turns its call of the function value into a call of
the literal, and the literal's call of f is a call of an inlinable function:
f went into f again on every one of up to 400 rounds. Before b29c306c8 such
functions had a cancellation check after the walk, whose landing pad kept
them out of the inlinable set; b29c306c8 removed those checks for closure
literals that cannot be canceled, and with them the pads.

## Fix

77cb1d2bf: inline_walk also walks the functions a body's values name
without calling them (closure literals, functions passed as values), for
cycles only. The golden internal/llvm/testdata/golden/inline_closure_recursion
is the case. When a change removes landing pads, checks or calls from
many functions, build compiler/xo cold with --phase-times before pushing:
the inliner's limits assume what those removed.
