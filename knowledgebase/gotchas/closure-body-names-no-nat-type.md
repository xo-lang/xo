# The checker has no type for a name inside a closure body seen from the enclosing function

Status: Observed (2026-10-21, Xo Lang 3, task.isolate direct call)

Symptom: Builder.isolate_direct (compiler/ssa conc.xo), which lowers
`task.isolate(fn() { h(q) })` as the call h(q) itself, never applied:
the assembly was byte for byte the old one and golden isolate_direct
kept its 109 allocations. A debug build that reported each guard's
return showed every site rejected at the type check of `h`.

Cause: `b.nat_check(e)` (the checker's type of an expression, nat_in
then type_in) returns -1 for an identifier inside a closure literal's
body when the builder is lowering the enclosing function: the checker
records those expressions' types for the closure's own body, which is
built later as its own function.

Fix: take the types from the SSA values instead: the variable the name
resolves to (`b.c.var_of(file, ident)`, which does work there) read with
`b.read(var)`, and `b.vty(value)` compared with the function type's
parameters and result. To find which guard rejects a site, have each
early return report a distinct `b.unsupported("...")` in a stage 1
build (`xo build --backend=arm64 -o X ./xo` in compiler/, about 70 s)
and build the program with it.
