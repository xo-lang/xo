# Profiler: a leaf pc at a function's start was charged to the function before it

Status: Observed (2026-10-16, profiler lines, decisions 0238 and 0239).

Symptom: `go tool pprof -top -lines` gave main 120 ms of flat time at
main.xo:11 for a program where fib takes all the CPU; the text summary
(which used the exact pc) said fib.

Cause: pprof's tools read every location as a return address and look
up pc - 1; prof.xo wrote the leaf's interrupted pc as is, so a sample
at a function's first instruction resolved to the end of the function
before it.

Fix: the leaf frame's pc is written plus 1 (prof.xo frame_pc), as Go's
runtime does, and every lookup (function and line) uses pc - 1.
