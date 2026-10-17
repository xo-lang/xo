# A borrowed projection that is consumed can make a moved list shared

Status: Observed (2026-10-12, rc-borrow agent, decision 0186); avoided by
rule 3 of the decision.

Symptom: the first version of borrowed projections (decision 0186) made
the golden borrow_proj do 3 more allocations on arm64 (234 against 230)
while every retain and release count went down. `main` called
`last_long(g.blocks[0].insts)` as the last use of `g`, and `last_long`
pushes onto its parameter.

Cause: before, `g.blocks[0]` was a counted projection: retained after
its definition, then `g` died (its last use was the index) and released
its list, so the `insts` list passed on was held once and the callee's
push wrote in place. Borrowed, the projection's uses borrow `g`, so `g`
lives until after the call; the argument is retained before it is
consumed and is shared during the call, so `reuse` copies it. Extending
an owner's liveness is free for reads but not for a value moved out of
an owner that would otherwise die first.

Fix: a projection is borrowed only if, at every instruction that
consumes it, the owner's root still has a use of its own on some path
after it (`usedAfter` in internal/ssa/rcborrow.go, `used_after` in
compiler/ssa/borrow.xo). Check allocation counts (XO_LEAKCHECK=1), not
only rc counts, when changing what the RC pass borrows.
