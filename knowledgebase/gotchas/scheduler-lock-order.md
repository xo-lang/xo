# The scheduler's lock order: a park's lock before the pool's

Status: Observed (2026-10-20, Xo Lang 2, decision 0286 step 1).

Symptom: xo built with main's body on a fiber hung in its own parallel
build (`xo build --backend=arm64`, 34 minutes with 1 s of CPU). A stack
sample (`sample PID 3`) showed four threads in pool.xo lock_slow: the
root in root_idle, a task's end in signal then wakep, main's fiber in
park_wait, and a worker in go_idle. A small program awaiting 16000 short
tasks from main (internal/llvm testdata/sched awaitstorm) hung in 4 of 6
runs.

Cause: root_idle held the pool's lock (the record's word 0) while it took
main's park's lock, to count a parked main as blocked; a task's end held
that park's lock in signal while push and wakep took the pool's. Two
locks taken in opposite orders on two threads.

Fix: the order is a park's lock, then the pool's, never the pool's then a
park's (written at the top of pool.xo). root_idle counts main blocked under
the park's lock alone and releases it before it takes the pool's. With
XO_LOCKCHECK=1 every pool lock taken through plock marks the thread's
worker record, and klock (a park's lock) with that mark set is a fatal
`xo: lock order` error at once: the inverted root_idle failed in 3 of 3
runs under it, where the deadlock showed in 4 of 6. The scheduler tests
run with it (internal/llvm TestMainFiberAwaitStorm, TestRootParkHandOff,
TestDeferredWakeBound). A new lock acquisition in pool.xo goes through
plock or klock.

## The root's loop spinning instead of stealing

Status: Observed (2026-10-20, Xo Lang 2, decision 0286 step 1).

Symptom: an rc_threads test program (internal/llvm testdata/rt
rc_threads, under ThreadSanitizer in the -short gate) was left orphaned at
100% CPU for 47 minutes; outside ThreadSanitizer, with ITER=20000 and
three or eight workers, it hung in 2 runs of 6 and took up to 27 s of user
time in the others (0.2 s with the fix). A stack sample showed the root in
root_idle, find_work, and dl_check, round and round.

Cause: work sat on a busy worker's ring. root_idle saw it (any_work),
took the root off the idle list, and returned; root_main's find_work was
called without spinning, which takes only the root's own queue and the
inject queue and never steals, so it found nothing and went idle again.
worker_main counts itself spinning when go_idle returns for work; the
root's loop did not.

Fix: root_idle returns 0 (work showed up: the root counts itself spinning,
P_SPINNING, and its next find_work steals), 1 (wakep woke it: it owns a
spinning count already), or 2 (main ended). Test: internal/llvm
TestRootNoSpin (testdata/sched stuckring: with one worker, a task waits in
a busy loop for a task it spawned, which only the root can steal; the
spinning root hung 20 runs of 20, the fixed one passed 20 of 20). rc_threads
alone did not make a good test: its wall and user time vary several fold
on a loaded Mac with or without the bug (before step 0 too). A scheduler
loop that goes idle on seeing work must look for it the way that saw it.

## A worker record kept across a park

Status: Observed (2026-10-20, Xo Lang 2, decision 0286 step 1).

Symptom: TestRuntimeTasks under ThreadSanitizer (the -short gate): a
plain read in pool.xo scope_join raced an atomic write in rq_put on
another worker.

Cause: a fiber can resume on another thread after any park (a task always
could; main can since step 1). scope_join read the thread's worker record
(TLS_W) once and then ran claimed tasks inline in a loop; a task that
parked moved the joiner, and the loop went on popping the old worker's
ring, which only its owner may touch (rq_get, rq_put, rq_put_batch at the
head and tail; thieves use rq_grab).

Fix: scope_join reads TLS_W again each round. With XO_LOCKCHECK=1 every
owner-only ring operation in pool.xo checks that the calling thread's
record is the ring's (owner_check, a fatal "ring owner" error): with the
old scope_join, rt/tasks at eight workers failed it in 3 runs of 5, the
fixed code in none.

The audit of every TLS_W and thread table read in std/native/llrt
(2026-10-20), for a value kept across a call that can park:

- pool.xo scope_join: kept across inline task runs; fixed (above).
- pool.xo push, grace_begin, defer_check, plock, punlock, klock,
  root_worker (poolsetup.xo), tracerec.xo ev: used at once, nothing parks
  in between.
- mutex.xo signal: kept across park_signal, which wakes another context
  and never parks the caller.
- tasks.xo spawn: kept until scope_add_at; in between only allocation,
  stores, a trace event, and sanhook.task_new, none of which parks.
- tasks.xo run_claimed: tw passed to sanhook.task_exit after the task
  runs; the TSan slot is the fiber's whenever there is a fiber (a fiberless
  root never moves), and the thread table is read again (te) for the
  spare context.
- sched.xo fiber_park: the table read again after the switch; fiber_run:
  on the scheduler's stack, which never moves.
- pool.xo futex scratch, worker_main, sysio.xo's clock: used at once.

A new read of TLS_W or rt.tlocal_addr() in the runtime that is used after
a call that can park reads it again after that call.

## Main blocked after its root slept

Status: Observed (2026-10-21, Xo Lang 2, decision 0286 step 1).

Symptom: TestCompileProperty seed 7 (-short) hung on arm64 where the Go
backend reported XO-F001: a program whose main ends in a receive nobody
sends slept forever about one run in six.

Cause: main's counted wait was counted blocked only by its root's idle
(root_idle). Main runs on any worker; when it parked there after the root
had already gone to sleep, nobody counted it, and the last worker to go
idle saw a live main and no deadlock.

Fix: every thread going idle (root_idle and go_idle) counts main's wait
(pool.xo count_main, main's fiber in the deadlock record's DLK_MAINF).
TestMainFiberDeadlock (testdata/sched/mainblock) hung 14 runs of 80
before, none after.
