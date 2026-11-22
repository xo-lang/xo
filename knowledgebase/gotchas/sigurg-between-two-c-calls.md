# The preemption signal landed in a C call made right after another

Status: Observed (2026-11-22, task #697, a tier's TestRuntimeCCalls on linux/arm64: `sleep: early 1 of 80`).

Symptom: a C `nanosleep` in a `use c` call returned early with EINTR beside CPU bound tasks, rarely
(1 of 80 sleeps of 100 ms in one tier run), although decision 0675 sends no preemption signal into a C
call. With 16,000 sleeps of 1 ms (the test program's `naps` mode), 9 runs of 12 had at least one on
each native backend (35 and 81 early sleeps, host 2).

Cause: the monitor (std/native/llrt pool.xo mon_scan) signals a worker whose task has run a slice
(W_RUN_AT_I) and whose in-C mark (W_CSEQ_I) is even. A task that sleeps in C, returns and calls C
again has an old run start, since the time in C counts, and is outside C for the moment between the
two calls. A tick in that moment set the ask and sent SIGURG, which arrived in the next call.

Fix: mon_scan sends SIGURG only when W_CSEQ_I equals the value the previous tick recorded (W_CSEEN),
so a worker that made any C call since the last tick is not signaled. The ask still goes out and a
loop head's poll serves it. A task that ran a whole tick without a C call and makes one just as the
signal is sent can still be hit (ADR 0675, addendum of 2026-11-22).

How to avoid it elsewhere: a "not in state X" check read by another thread says nothing about the
next instant. When the cost of acting wrongly is not recoverable (a signal cannot be taken back),
ask for the state to have been steady for a period, not only to hold now.
