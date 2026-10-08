# A spawn heavy native program spent most of its time in the kernel

Status: Observed (2026-10-08, std/llrt task runtime, decision 0068)

Symptom: a program of 200 rounds of 1,000 tiny tasks built with
`--backend=arm64` took 1.0 s of wall time with 0.56 s user and 4.4 s
system time on 12 workers (the Go backend: 0.15 s), but 0.08 s with
`XO_THREADS=1`. `sample <pid>` showed `__psynch_mutexwait`,
`__psynch_cvwait`, and `__psynch_cvsignal` on top.

Cause: on Darwin a contended pthread mutex uses the fair share policy:
the unlocking thread hands it to the first waiter through the kernel,
so every contended lock and unlock is a system call, even for critical
sections of a few instructions. Every idle worker also slept on a
condition variable and was signaled for each spawned task.

Fix (2026-10-08): runtime mutexes are initialized with
`PTHREAD_MUTEX_POLICY_FIRSTFIT_NP` (`mu_init` in xo_rt.c; Linux keeps
the default); the deques, the cancellation tree, and the deadlock
counters use a spin lock that yields after 64 tries; an empty deque is
skipped without its lock; up to two idle workers spin for work before
they sleep and a spawn skips the wake up while one does. System time went
from 4.4 s to 1.3 s (wall 1.0 s to 0.33 to 0.48 s). What remains is
contention on the per scope and per task mutexes and parks of awaiting
contexts; a lock free deque (Chase and Lev) and atomics for a task's
state transitions are the next steps if spawn heavy code matters.
Timings are indicative (single runs on a shared host).
