# ThreadSanitizer crashes on deep recursion

Status: Observed (2026-10-08, internal/llvm TestGoldenThreads)

Symptom: the golden programs `recursion` and `rc_closures`, built with
`-fsanitize=thread`, die with `ThreadSanitizer:DEADLYSIGNAL`, a BUS
error, or `Killed: 9`, even on one thread and with a 1 GiB stack. The
same IR without TSan runs fine on four threads.

Cause: both recurse hundreds of thousands of calls deep (a depth
counter, a long box chain built recursively, closures capturing
closures). TSan keeps per thread shadow state for the stack and its own
call stack; at that depth it runs out (an internal limit, not a race in
the program).

Fix: `TestGoldenThreads` lists them in `deepRecursion` and runs them on
four threads without TSan (still with the leak check); every other
golden program that exits normally runs under TSan. A new golden with
deep recursion that fails the same way belongs in that list; check
first that it fails on one thread (`-DXO_RT_TEST_THREADS=1`) too.
