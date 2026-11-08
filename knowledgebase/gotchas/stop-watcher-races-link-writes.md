# A SIGINT left a build's hidden stage when the link was still writing it

Status: Observed (2026-11-08, Xo Lang 2: the hourly at c6b3a9d6b, a default
native build)

## Symptom

cmd/xo TestBuildStagedOutputs failed once in the hourly -short run at
c6b3a9d6b: `a SIGINT left [.../out/.int.tmp-TAG-RAND]`. Alone it passed
85 of 85 runs.

## Cause

On a stop, the watcher (compiler/xo/ld.xo watch_stages) removed each stage
it had been told about with one `remove_all`, and ignored the error. The
stage is announced before it is made, so an unannounced stage is not the
gap. The link writes into the stage while the watcher removes it: ld64 and
dsymutil are child processes the SIGINT does not reach, and xo's own
writer runs on another task. A file made while `remove_all` walks the
directory leaves it not empty, so its last rmdir fails, and xo then exits
130 with the stage still there.

## Fix

remove_stage renames the stage to `DIR/.xo-stop.NAME.tmp-TAG-RAND` first. After that,
nothing makes a path in the stage by name any more, and a child holding an
open file writes into the tomb. It then removes the tomb up to 50 times,
2 ms apart, until it is gone. The test also fails on a `.xo-stop*` left
behind.

--dsym makes PROG.dSYM's directories in the stage (ld.xo), and cc_dsym's
dsymutil (cc and LLVM links) is a child that outlives xo's exit and makes
its bundle's directories itself, so either could make a renamed stage
again. Both run between stage_enter and stage_leave (ld.xo StageStop): a
step counts itself busy and then reads the stop flag; the watcher sets the
flag, waits up to 2 s for busy to drain, and only then removes the
stages. TestBuildStagedOutputs runs its SIGINT check with --dsym too.

The rename has a race of its own: the build's task finds its stage gone
(its write of NAME.tmp fails) and exited 1 through fail_stage while the
watcher still removed the tomb, which then stayed (6 runs in 300 under
load, exit 1, "cannot write the program to .../.int.tmp-.../int"). Once
the watcher has begun, Stage.abandon now waits for its exit 130 instead,
and a build of an output removes the `.xo-stop.NAME.tmp-*` tombs an
earlier process left beside it (ld.xo remove_tombs). With both, 300 of
300 runs passed, plain and with --dsym, nothing left.
