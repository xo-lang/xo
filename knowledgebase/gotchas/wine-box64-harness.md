# Wine under Box64: a long lived wineserver and Box64's own output

Status: Observed (2026-10-18)

## Symptom

A run of every golden (windows/amd64, LLVM) under Wine with Box64 in
Docker (scripts/docker-wine.sh box64), one program after another in one
container: 94 of the first 130 passed, then every program, `strings`
and `structs` included, ran into the timeout; earlier ones failed with
no output at all and odd exit statuses (overflow_add: exit 29 and empty
stdout, where it prints three lines and exits 70). Run alone in a fresh
prefix, each of them passed. A second run that reset the wineserver
after each program still failed 36 programs, each with extra lines in
its standard output: `[BOX64] Error initializing native
libgnutls.so.30 ...`. And env_case and net_only saw no PATH at all
(`cmd /c echo %PATH%` printed `%PATH%`) in the prefix the first run had
damaged.

## Cause

Three harness effects, no Xo bug: one long lived wineserver degrades
after crashes and hangs (Box64's "Illegal instruction" and segmentation
faults in Wine's own processes) until nothing runs; Box64 writes its
errors to standard output, and every program that uses bcrypt makes
Wine load gnutls, which Box64 fails to wrap; a prefix whose first
wineboot ran while that state was broken lacks the Environment keys.

## Fix

scripts/wine-goldens.sh: a fresh WORK directory (so a fresh prefix) per
run, `wineserver -k` and `wineserver -w` after each program, Box64's
lines dropped from each program's standard output (the exit status
kept), a 60 second limit. With it, every failure left is a known Wine
gap (toolchain.md) or Box64 flakiness: about six programs a full run
crash inside Wine's own code ("Unhandled illegal instruction at
00006FFF...") and pass when run again; rerun a failure alone before
calling it a bug.

## A prefix whose wineboot stopped part way (Observed, 2026-10-19)

Symptom: a run of every golden reports `ok 0 of 202`, every program
`exit 53` with no output and no stderr, while the 31 golden subset with
the same compiler passes, and each failing executable passes when run
alone in another directory. Twice in a row on 2026-10-19, with the Mac's
load at 20 to 48.

Cause: the run's first `wineboot -u` under Box64 stopped part way
(services.exe "c000001d", an illegal instruction, in Box64): the prefix
had 419 files in system32 where a good one has about 787, and every
`wine` in it exits 53, `wine cmd /c echo` included.

Fix: scripts/wine-goldens.sh's run.sh probes the prefix after wineboot
(`wine cmd /c echo xo-ok`) and makes it again, up to three times. When a
run's summary shows every program failing with the same status, check
the prefix before the programs.
