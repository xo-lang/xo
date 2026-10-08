# RC verifier: paths into an `unreachable` block disagreed

Status: Observed (2026-10-08, `internal/ssa/verify_rc.go`, SSA golden
`failing`)

Symptom: after `InsertRC`, a `match` over nested enums failed
verification with `edge to b12 holds {v62:1}, another predecessor {}`.
b12 was the `unreachable` block a match falls into when no arm matches
(the checker proved it exhaustive). One arm test had retained a
payload (`v62`) before the inner test failed; the other predecessor held
nothing.

Cause: InsertRC puts no release on edges into blocks that end in
`unreachable` or a fault (the program stops there, and a release before
`unreachable` cannot be checked; see `rc-verifier-loop-param-leak.md`).
The verifier still required every predecessor of every block to agree
on what is held, including those blocks. Programs before failing
functions never had a heap payload retained on a path into the match's
fall through block.

Fix: for a block that ends the program, the verifier merges the incoming
states (the largest count per leaf) instead of requiring them equal.
Nothing is released there, so there is nothing to balance; uses in the
block are still checked against the merged state.
