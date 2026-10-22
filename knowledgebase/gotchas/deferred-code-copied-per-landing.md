# Deferred code copied at every landing grows a function exponentially

Status: Observed (2026-10-21), fixed the same day

Symptom: `TestDeferFaultProperty/8` (internal/gen) timed out after 12
minutes in the hourly run, and its arm64 build of the generated program
kept running after the test binary died, growing to 30 GB. Alone, the
build sat in the rc pass (`merge_jumps` calling `remove_blocks`) for
more than 10 minutes. `xo dev dump ssa --fn=case2 --stage=build` showed
225,753 blocks for a 60 line function (deferred blocks nested three
deep, with calls that can fault inside and around them); one `println`
of the deferred code was lowered 15,444 times.

Cause: the SSA builder lowered the pending deferred code again at every
landing: each operation that can fault in a block with deferred code got
its own copy of every enclosing block's deferred code (`unwind`), and
each fault in top level deferred code its own copy of the blocks opened
inside that code (`flush_landing`). The copies hold landings of their
own, so nesting multiplies them. `merge_jumps` then renumbered the whole
function once per merged block, quadratic in that count. The orphan:
internal/xotool ran `xo build` with a bare `exec.Command`, and a
`go test -timeout` panic runs no cleanup.

Fix: an unwind of ordinary code jumps to a shared block per open block
and its deferred statements (compiler/ssa `unwind_block`, keyed by
`unwind_key`: the statements' positions, since every landing snapshots
the frames), chained down to the scope's or the function's unwind block;
a fault in deferred code jumps to one shared block per statement and
open blocks (`defer_fault_block`). Both are sealed with the fault
continuations, so they read variables through their predecessors; for
that `flush_landing` now leaves the faulting operation's block with the
definitions from before the operation and gives the later ones to the
continuation block (landing-reads-later-definitions.md). case2 went to
4,288 blocks, the build to 6 s and 0.87 GB, output the same as the Go
backend's. xotool's `xo` commands run behind `testtmp.Guard`.
`TestDeferFaultBlocks` bounds case2 at 8,000 blocks.
