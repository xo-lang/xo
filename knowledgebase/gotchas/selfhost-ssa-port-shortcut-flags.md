# selfhost/ssa: a shortcut flag of the port encodes the stage it was written in

Status: Observed (2026-10-11, SSA stage 5)

Symptom: after porting scopes, 78 functions of programs with `scope`
differed from the Go builder only by a missing landing:
`call rt.task_spawn, ... lands v21` in Go, no `lands` in Xo, and the
block numbers after it shifted.

Cause: the Go builder's `add` gives every operation that may fault a
landing when `lexicalCleanup()` holds (a block with deferred code or an
open scope around it). The port added `S.any_defer`, set by a `defer`
statement, and skipped that test while it was false: correct while
only `defer` made cleanup (stage 4), wrong once scopes did.

Fix: opening a scope sets `any_defer` too (conc.xo `new_scope`), and
the field's comment says "a `defer` or a scope". When a stage adds a
construct, look for flags and fast paths of earlier stages that stand
for "the only construct that can need this" (`grep` the condition the
Go code tests, here `lexicalCleanup`).
