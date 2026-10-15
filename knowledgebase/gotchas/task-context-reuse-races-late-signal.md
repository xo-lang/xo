# Reusing a task's context races a waker still signaling its park

Status: Observed (2026-10-15, a perf change that was not merged); the
cause is suspected, not proven

Symptom: tasks.xo run_claimed was changed to keep its 320 byte context
as the thread's spare and reuse it for the thread's next task run
(instead of a fresh block per run, freed at the end). Programs passed
the goldens, but an xo built with the change exited with status 137 and
no message on every `xo build` (not under lldb, where it passed), and
one full LLVM golden run faulted fs_ops_edges with a false XO-F001
deadlock. Without the change both were fine.

Suspected cause (first guess, see below): a waker can still be inside pool.xo park_signal for a
context's park (holding the park's lock word, about to set signaled,
clear counted, or queue the fiber) when the waiting task has already
seen its condition, finished, and left run_claimed. With a fresh block
per run that late write lands in a freed block (silent); with reuse the
next run's mem_set and park_reset clobber a lock someone holds, or a
counted root's deadlock count is changed by a signal meant for the old
run. The C runtime kept the context on the fiber's stack, which the
fiber cache also reuses, so the same window may exist there.

Tried (2026-10-15), neither fixed it: taking and releasing the park's
lock word at the run's end before keeping the context (xo build still
137), and taking the spare from the Xo heap instead of rt.heap_large
(133 and 137). So the late writer is not only park_signal's lock, and
the allocator is not the cause; other references may outlive the run
(a park left registered as a node's waiter, a channel or wait list
entry in the context). With a freed block those writes are silent,
which makes them worth finding even without reuse.

Fix: none; the change was dropped. Before reusing a context, find every
reference to it that can outlive run_claimed (XO_LEAKCHECK and an
AddressSanitizer build of xo with the reuse would show the writer).
