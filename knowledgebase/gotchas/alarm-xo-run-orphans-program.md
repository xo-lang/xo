# A timeout on `xo run` leaves the program running

Status: Observed (Xo lang 5, 2026-10-30)

## Symptom

A try script ran a hanging server program with `perl -e 'alarm 60;
exec @ARGV' xo run --backend=arm64 DIR`. The alarm fired and the script
went on, but the program (`~/Library/Caches/xo/build/KEY/arm64/bin/NAME`)
stayed alive with PPID 1, holding its listener. Its heavy.sh job also
held a gate slot for 40 minutes, because the job's pipe stayed open.

## Cause

`xo run` builds the program and starts it as a child. SIGALRM ends `xo
run` itself, not its child, so the child is reparented to launchd and
keeps running. macOS has no timeout(1) with `-k` and no process group kill
from perl's `alarm`.

## Fix

- Build, then run the binary directly under the timeout, so the timeout
  signals the program itself: `xo build --backend=arm64 -o BIN DIR`, then
  `perl -e 'alarm 60; exec @ARGV' BIN`.
- On the Linux offload hosts use `timeout -k 5 90 xo run ...`, which
  signals the whole process group.
- Do not run a program on a backend that is known not to finish it (a
  Go only golden on native) outside a timeout that reaches the program.
- A leftover found afterwards is stopped by its PID only by the session
  that started it (CLAUDE.md safety rules), or reported to the lead.

## Follow-up (2026-10-30): a Go backend binary ignores the alarm

Status: Observed (2026-10-30)

### Symptom

`perl -e 'alarm 15; exec @ARGV' ./prog` ended a native (arm64, LLVM)
Xo program after 15 s but not one built with `--backend=go`: a hung Go
backend probe ran until heavy.sh's 1200 s stall reaper stopped the
whole gate.

### Cause

The alarm survives exec and delivers SIGALRM, whose default action
ends a process. The Go runtime installs its own handler for SIGALRM
(os/signal semantics: delivered to a channel nobody reads), so the
signal is caught and ignored.

### Fix

Bound a run with a watchdog that kills the PID it started:
`prog & pid=$!`, poll `kill -0 $pid` for the limit, then
`kill -9 $pid` (the PID captured at launch, never a name or pattern).
macOS has no timeout(1). Example:
~/xo-lead/notes/xo-lang-5/scripts/pipe-probe2.sh.
