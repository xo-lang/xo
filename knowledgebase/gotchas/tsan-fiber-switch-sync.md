# ThreadSanitizer saw no race between tasks on one thread

Status: Observed (2026-10-20, Xo Lang 2, while planting races for
decision 0273's native --race).

Symptom: a data race planted between four tasks (a plain load and
store of one shared word in each) with one worker thread (XO_THREADS=1)
was reported only between one task fiber and the worker thread's own
context (2 reports), never between two task fibers; with the fix, 20
reports, most between fibers. With two tasks the race still showed (one
of them ran on the worker's context), so a two task test passes on both:
the test needs a race between two fibers. --race builds (LLVM) had this
since the fiber hooks went in.

Cause: the runtime's fiber hooks (std/native/llrt/sanhook.xo hook)
switched with `__tsan_switch_to_fiber(fiber, 0)`. Flag 0 makes the
switch a synchronization: everything the old fiber did happens before
everything the new one does, so the fibers of one thread were totally
ordered. Go's race detector does not order goroutine switches.

Fix (decision 0273): two modes. --race (mode 2) switches with flag 1,
`__tsan_switch_to_fiber_no_sync`, leaves the runtime uninstrumented
(its per thread state, which the fibers of one thread hand on with no
ordering ThreadSanitizer sees, made a false race at every structure
once switches stopped synchronizing), gives each task a context of its
own made at its spawn (a fiber's creation synchronizes with its creator:
made when the task started running, it ordered the awaiting parent's
racing write before the task), and releases and acquires at every
synchronization point; internal/llvm TestRaceSyncTable tests each both
ways and TestTSanSameThreadRace (four tasks, one thread, a report between
two fibers) fails with flag 0. The runtime's own race tests keep flag 0
and the instrumented runtime (mode 1, XO_TSAN_RUNTIME=1 and
xo_tsan_runtime linked).
