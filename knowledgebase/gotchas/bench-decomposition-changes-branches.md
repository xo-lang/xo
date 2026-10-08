# Cutting a benchmark into parts can change its branch behavior

Status: Observed (2026-10-08, bench sort, arm64 against LLVM)

Symptom: to find where sort spent its time, a copy of the program
without the `quicksort` call ran its generator and check loops; the
native binary took 78M cycles against LLVM's 44M, a gap larger than the
whole program's.

Cause: without the sort, the check loop's `if less(xs[i], xs[i - 1])`
runs on random data and mispredicts half the time. The native backend
branches there, LLVM computes the flag with `csel`; on sorted data (the
real program) the branch always goes one way and costs nothing. A
variant that added `&& i < 0` to keep the loop let LLVM delete the call
entirely.

Fix: when splitting a program to attribute time, keep every branch's
data as it is in the full program (remove whole loops, not the work
that shapes their inputs), and compare parts by subtraction: full minus
"generate only" minus "no check loop". For sort that gave generator 47
against 37M cycles, sort 833 against 821M, check 12 against 5M.
