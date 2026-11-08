# macOS charges 150 to 200 ms to the first exec of every new binary

Observed 2026-11-08 (Xo Lang 2, on the development Mac, darwin/arm64,
`spctl --status`: assessments enabled), while measuring the REPL's
rebuild and rerun loop (decision 0572).

## Symptom

`xo run` after a one-line edit of fib took 273 to 350 ms, though the
build took 66 to 80 ms and the program runs in 5 to 8 ms. Running each
newly built binary took 146 to 236 ms; running the same file again took
5 to 8 ms.

## Cause

macOS assesses each executable file the first time it maps code from
it. Every new file pays, whatever made it and whatever it holds:

- a fresh C `int main(){return 0;}` from cc: 180 to 197 ms first, then
  3 to 4 ms;
- copies of one Xo binary (same bytes, new inodes): 146 to 160 ms each;
- `dlopen` of freshly built C dylibs in a running process: 161 to 199
  ms each.

Rewriting the old file in place (same inode) skips the assessment, but
the kernel's cached code signature then kills a real binary: Xo
binaries of different sizes written over each other died with SIGKILL
(`Killed: 9`) on every run. A tiny C pair happened to survive. So
writing in place is not a fix.

The cost is most likely XProtect's scan of new code (not traced to a process yet). See
gotchas/xprotect-false-positive-empty-list.md, for a scan that blocked
a binary, and gotchas/amfi-gatekeeper-macho-layout.md, for exec waiting
on Gatekeeper; this entry is the cost of the scan when it passes.

## Fix

None inside xo, so far. For timing, run a binary once before measuring
it, or measure on Linux. A loop that runs new code on every step, such
as a REPL or `xo run` in an edit loop, pays the cost each step on macOS
unless it runs no new file: an interpreter, or code in memory.

Next experiment (needs the owner, since it is a system setting):
1. Add the terminal app under System Settings, Privacy & Security,
   Developer Tools.
2. Rerun the fresh C binary test.

Apple's tools exempt processes started from a developer tool from this
assessment, which is believed to remove the delay. That is not yet
measured here.
