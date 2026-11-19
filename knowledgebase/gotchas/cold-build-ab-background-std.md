# A cold build --ab in a fresh HOME per round piles up background std builds

Status: Observed (xw-1, 2026-11-19)

Symptom: `xo bench --ab` of a cold `xo build` (aws examples/presign, XO_DEP_LAZY=0 against on,
task #373) on host 1 gave wall B/A 0.98 at 4 rounds and 0.929 at 10, with spreads of 13 to 22%
and a load that rose every round, from 11 to 31 on 12 cores, though nothing else ran there.
Every round was refused as loaded until --keep-loaded.

Cause: each side wipes its HOME (and so the std cache) before every round, and a cold build in an
empty cache starts a detached `xo dev stdcache` (std's variants and the warm build,
compiler/xo/stdcache.xo) that runs on after `xo build` exits. Each round started another, so the
later rounds ran beside several background std builds from earlier ones, A's and B's mixed.
`ps` after three builds showed six `xo build ... .stdobj-prog` children still at 60 to 300% CPU.

Fix: set XO_SEPCOMP_BG=0 on both sides of a cold build timing (heavy.sh jobs already do; a script
sent through offload.sh does not). The same run then had load ~3, spread ~1%, wall 0.907 (0.903
to 0.915). The timing then leaves out the background fill a user's machine does after the build,
as heavy-runs-no-background-std-fill.md describes; say so beside the number.
