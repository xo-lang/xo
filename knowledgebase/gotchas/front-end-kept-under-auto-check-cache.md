# The front end stayed alive through the passes in default builds (2026-10-23 to 2026-10-26)

Status: Observed and fixed (dd9e4a954); regression test TestFrontEndFreedByDefault.

## Symptom

An edit rebuild of llvm-fib peaked at about 300 MB (Go's 67 MB) and the live heap at the reference
counting phase was 141 MB of parse trees and checker data nothing read (XO_PROF=heap with
XO_PROF_HEAP_AT=insert_rc: parser.parse_file 74 MB, Checker.declare 18, parser unparenthesized_effect
13, source bytes 13), freed only when the check of assumptions ran (12 MB live after it).

## Cause

Decision 0352's strip (lower lets the checker go once the SSA is built) required
`!c.mod_skip_auto`, since the module check cache's retry reads the checker after lowering. The owner
made `XO_CHK_CACHE=auto` the default on 2026-10-23, so strip was false in every default build; nothing
failed, a build only held 130 MB longer. XO_CHK_CACHE=0 (or off) still stripped, which hid it from
anyone who measured with that.

## Fix

link_view keeps what the retry reads (mod_skip, mod_skip_auto, skip_unsafe) and strip no longer asks
for `!mod_skip_auto`. The retry checks the program again from scratch, so it needs no more of the old
checker. A default build now prints `phase front_end_freed` under --phase-times (the marker the test
reads) and the live heap after it is 29 MB (12 MB at insert_rc). llvm-fib edit rebuild, 11
interleaved reps: peak RSS 298 MB to 278 MB, wall 320 ms both. The peak is now where the front end is
built (check, fe.build and ssa: 272 MB mapped at the end of ssa), so a lower peak needs less front end
(decision 0380) or less churn, not an earlier free.

## How to see it again

`xo build --phase-times`: `phase front_end_freed` between `tz` and `prune_unwind`; its absence is the
bug. TestFrontEndFreedByDefault (cmd/xo/frontfree_test.go) builds twice by default and once with
XO_KEEP_FRONT=1 and checks both; its negative control (the old condition put back) fails.
