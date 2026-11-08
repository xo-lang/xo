# The build watchdog killed xo and left its clang running

Status: Observed (2026-11-07)

## Symptom

The lead looped internal/llvm TestGoldenSplit/pass_const_edges with
XO_BUILD_LIMIT set. Each build that passed the limit was reported as
"xo did not finish within XO_BUILD_LIMIT", and the clang -cc1 it had
started kept running at full CPU after it. Three of them ran for 11
minutes until the lead stopped them by PID.

The hang report was also misleading. It showed xo's main fiber "blocked
in network I/O" on clang's stdout pipe, so the run read as a hang of xo's
poller. In fact xo was correctly waiting on a clang that was still
working.

## Cause

xotool.RunWatched killed only the xo process (`cmd.Process.Kill`).
testtmp.Guard runs xo behind the lifeline wrapper, which makes xo the
leader of its own process group. But the wrapper kills that group only
when the test process's lifeline ends, which is at the end of the test
binary. Until then, the children of a killed xo (clang, a linker) stay
alive as orphans.

The hang report listed xo's descriptors and stacks, but not its child
processes.

## Fix

- RunWatched now kills the whole process group (internal/xotool
  killgroup_unix.go: SIGKILL to -pgid, only when the child leads a group
  other than the test's).
- HangReport now starts with the child's children: pid, elapsed time,
  CPU time, RSS and command line.
- Test: TestRunWatchedKillsTheGroup. It fails without the fix: the
  grandchild outlives the kill.
- When a build hangs, read the children section first. A child with
  growing CPU time is a slow tool, not a hang of xo.
