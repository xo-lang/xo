# Under Box64 Wine a program's exit code is noise

Status: Observed (2026-10-28)

## Symptom

A stripped windows/amd64 llvm-fib (decision 0388) exited 139 (segmentation
fault) under scripts/docker-wine.sh box64 after printing the right result,
which looked like a regression of the change being gated.

## Cause

Box64's Wine does not exit cleanly: the same normal windows/amd64 fib,
run 8 times in a row, exited 29, 0, 1, 0, 132, 29, 132 and 29, always after
printing `fib 102334155`. The program's own output was right every time.

## Fix

Under Box64 Wine judge a program by its output (want.txt's stdout), not its
exit code; run a changed build and the normal build side by side several
times before calling a difference. Exit codes, threads, signals and
suspension need a Windows CI run (gotchas: Wine is single-threaded smoke
only, owner 2026-10-21).
