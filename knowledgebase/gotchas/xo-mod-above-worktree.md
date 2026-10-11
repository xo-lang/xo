# An xo.mod in the main checkout captures every worktree under it

Status: Observed (2026-10-11)

Symptom: in a `.claude/worktrees/<name>` checkout, `go test -short`
fails in places the change did not touch: `internal/gen` TestHoles and
the `map_key_fault` golden print positions as
`.claude/worktrees/<name>/internal/gen/testdata/...` instead of
`main.xo`, and TestSelfhostChecker reports hundreds of runs whose module
path is `github.com/acme/billing/.claude/worktrees/<name>/bench/...` on
the Go side only.

Cause: worktrees live inside the main checkout. Module discovery walks
up from each directory to the nearest `xo.mod`; an untracked `xo.mod`
left at the main checkout's root (here `module github.com/acme/billing`
with a `billing.xo`, created by something run there) made every
directory of the worktree a package of that module. The self hosted
checker, given the repository root, did not walk above it, so the two
checkers disagreed.

Fix: do not leave `xo.mod` files in the main checkout's root (run `xo
init` experiments in a temporary directory). To test a worktree while
one is there, copy it outside the checkout (`rsync -a --delete
<worktree>/ <scratch>/wt/`) and run the tests in the copy; the results
then match a clean checkout.
