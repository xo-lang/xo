# A test task that spins for its child can run the child on its own thread

Status: Observed (2026-10-21 alloc_threads' waves, 883c1d515; 2026-10-22
TestRuntimeTasks' magazines, Xo Publish).

## Symptom

A runtime test whose task spawns a child and spins (`while done.get() ==
0 {}`) so that another worker must take the child fails now and then
under load: `check failed: magazines: the blocks were freed on their own
thread` (TestRuntimeTasks at XO_THREADS=3: 2 of 16 runs under
`go test -count=16 -parallel 8`, 2 of 40 rounds each), and earlier
`waves: 1 freed on the allocating thread` (TestRuntimeAlloc).

## Cause

Preemption: decision 0286 step 3's loop polls and decision 0318's
signal yield a fiber that ran 10 ms while work waits. The spinning task
is such a fiber; once it yields, its own worker may take the child, so
that round's work runs on the allocating thread. Correct scheduling,
wrong test assumption. With XO_PREEMPT=0 the same stress passed 32 of
32 runs.

## Fix

Check that at least one round ran on another thread, not every round
(the magazines' real check, the chunks carved, stays per test). Any new
test that spins to force work onto another worker needs the same: count
the rounds, or run with XO_PREEMPT=0.
