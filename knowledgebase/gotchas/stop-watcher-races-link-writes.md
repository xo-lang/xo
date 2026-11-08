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

Not covered yet: --dsym makes PROG.dSYM's directories in the stage
(ld.xo), and cc_dsym's dsymutil (cc and LLVM links) is a child that
outlives xo's exit and makes its bundle's directories itself. Either can
make the renamed stage again. A follow-up adds a bounded wait on an
in-flight count around cc_dsym and a stop flag the --dsym mkdir checks.
