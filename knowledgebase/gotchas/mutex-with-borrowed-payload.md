# Mutex.with copied its list on every push: the payload was a borrowed projection

Status: Observed (2026-10-30, Xo Lang 2's report, compiler/ssa/borrow.xo)

Symptom: pushing one value per `m.with(fn(var l) { l.push(x) })` was
quadratic on the native backends and LLVM: 200,000 pushes took 31.4G
instructions on arm64 against 17.9M for a plain loop. `xo dev dump ssa
--fn=F --stage=rc` showed `retain` of the payload before the closure's
`callv` and a `release` of the tuple's Option field after it, so the
list had two holders at the closure's first push.

Cause: Mutex.with's packed path (compiler/ssa/conc.xo mutex_with) takes
the value out with `list.pop` (a `(List[T], T?)` tuple), reads the
payload of field 1, calls the closure, and pushes the result onto field
0. borrow.xo's decide_borrow keeps a projection counted only when its
owner is dead at the projection's consume (used_after). Two things made
it see the owner as live: used_after counted uses of any leaf of the
root, so the push onto the tuple's field 0 after the call counted for
the payload's field 1; and its walk around the loop, which stops at the
root's own block (a use there is of the next value), did not stop when
that block was the consume's own, so it found the payload's own read
earlier in the block. The payload became a borrowed projection, rc.xo
never offered it to movable(), and the caller retained it.

Fix: used_after takes the owner leaf and counts only uses of that leaf
or of one that holds it or that it holds (Borrows.all_p, paths_overlap),
and reaching the root's block always ends the walk. Rollback is
unchanged: the packed closure hands back the value it was given on an
error or a fault (build.xo packed_value, finish_fn), and the closure's
own counts keep that value alive (and copy on write) until its last
point that can fail. Guarded by internal/arm64 TestMutexWithPushLinear
(an instruction ceiling at 200,000 pushes) and the conc_mutex_move and
conc_mutex_rollback goldens.
