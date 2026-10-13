# ThreadSanitizer does not see synchronization done in Xo code

Status: Anticipated (2026-10-13, decision 0212 stage 2); the annotations
below are in place and TestGoldenThreads passes with them on darwin/arm64.

Symptom (expected without the annotations): false data race reports
between C code on two threads whose only happens-before edge is a lock,
a park, or a queue that std/native/llrt/pool.xo now runs: a task's
fields written by its spawn and read by the worker that runs it, a
scope's tasks freed by the joiner after the workers' last touch, data
read after a park wait that a signal ended.

Cause: only the C runtime is compiled with `-fsanitize=thread`; the Xo
runtime modules are not instrumented, so their atomics (the lock words,
the parks' signaled words, the run queues) are invisible to
ThreadSanitizer. Before stage 2 the C's own lk/unlk around each park
gave it the edge.

Fix: the C tells it at the boundaries (xo_rt.c TSAN_RELEASE and
TSAN_ACQUIRE, `__tsan_release` and `__tsan_acquire`): park_signal
releases the park before it calls Xo, a park wait and a scope join
acquire it after; a spawn releases the task, and the C that runs it
(xo_sched_run_task, the help in a scope's close) acquires it; a lock
whose slow path ran in Xo is followed by an acquire load of its word in
C, which ThreadSanitizer pairs with the unlocker's release. Any new
hand off through Xo code needs the same pair.

Related: on Linux a raw `mmap` system call bypasses ThreadSanitizer's
interceptor, which keeps mappings in its application ranges, so fiber
stacks come from the C library's mmap when the worker's hooks word is set
(sanitizer builds and XO_RTSTATS; pool.xo map_stack, RT_MAP). Not
observed yet: Linux runs at the end of stage 2.
