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

Developer Tools listing (measured 2026-11-08): it does not remove the
cost. The terminal apps were already listed under System Settings,
Privacy & Security, Developer Tools (`DevToolsSecurity -status`:
"Developer mode is currently enabled"). The fresh C binary test
(~/xo-lead/notes/xo-lang-2/fe.sh: five new `cc` binaries, each run
twice) still paid on its first run:

- the owner's own terminal session: 148.9 to 164.3 ms first, 3.0 to 6.9
  ms again;
- an agent's shell (under Crosswalk's ptyhost): 181 to 186 ms first, 3
  to 8 ms again;
- the same through heavy.sh (heavyd's job): 182 to 251 ms first, 2 to 4
  ms again.

So how heavyd launches jobs is not the cause, and the exemption is not
the fix.

Next experiments, ranked (measure only, no setting changes):
1. Code with no new file: a long-lived runner maps an entry's linked code
   into MAP_JIT memory (pthread_jit_write_protect_np) and calls it. Xo's
   binaries are not hardened runtime, so no entitlement should be needed.
   Time a tiny C runner's load against the 155 ms.
2. An ad hoc re-sign (`codesign -s - -f`) of a binary rewritten in place:
   whether it avoids both the SIGKILL and the assessment.
3. A binary on another volume type (a read-only RAM disk from hdiutil).
