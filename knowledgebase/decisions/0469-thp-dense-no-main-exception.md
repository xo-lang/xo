# 0469: XO_THP=dense without the main thread's exception

Status: Accepted (owner, 2026-11-03; amends decision 0456; proposed by Xo Lang 2, 2026-11-03)

## Context
Decision 0456 (v4) advises the main thread's heap record's first 4 MiB chunk MADV_HUGEPAGE
whole, since a build's front end allocates there densely from the start; v3 without the
exception cost coll edit 1.034 against v4's 1.017. Since then e68bf2fac (lists and strings
reserve 4n) and f3f9552ac (the large block cache bounded by the bytes in use) changed how the
main record fills. On linux/arm64 host 1 (THP always, defrag madvise), 2026-11-03, coll putget's
max RSS at the default is 10,704 KB against v0.35.0's 10,352, all of it huge pages (5,604 off).

## Decision
The main record's first chunk opens by halves like every other record's: heapos.reserve drops
the is_main_record branch.

## Consequences
- putget max RSS (dense) 10,704 to 8,696 KB: 0.84 of v0.35.0 (kernel, off unchanged).
- ab-host 5 rounds against f3f9552ac: coll edit wall 0.989, CPU 0.983, RSS 1.034; self wall
  1.002 (0.998 to 1.005), CPU 1.004, RSS 0.995; all within noise.
- A server's main thread no longer costs a chunk of huge pages over off (0456's consequence).

## Alternatives
- Keep the exception (0456 v4): 2 MB per small program for no measured build speed now.
- Advise only the main chunk's second half up front (2b531fa63): same putget RSS; build runs:
  see ab-thp2*.txt. More code for the same result.
