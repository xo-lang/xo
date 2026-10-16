# ThreadSanitizer: evloop's poller against a record's earlier owner (kqueue)

Status: Observed (2026-10-16), explained and fixed the same day

## Symptom

TestRuntimeEventLoop built with `--race` (every function marked,
524edb0c) failed about one run in three under load: the poller
(poll_main, through pd_ready: the descriptor's lock, its ready flag)
against an earlier access by another thread to the same address
(cancel_locked's read of a task node, fiber_free's read of a fiber
record). It reproduced within a few rounds of four concurrent copies.

## Cause

Not a use-after-free (a debug liveness check of contexts, of parks
before pd_ready signals them, and of descriptors freed with an
operation in flight never tripped in 40 rounds): the descriptor record
was a block the runtime's malloc had handed out before (a fiber's, a
task's), and the only order between its new owner's writes and the
poller's accesses is the kernel's, a kqueue registration before its
events. ThreadSanitizer cannot see that edge through a raw kevent, so
it saw the poller race the block's previous owner. Linux had the
annotation already (gotchas/tsan-raw-epoll-no-edge.md); macOS's kqueue
path did not.

## Fix

poll.xo register (darwin) releases ev() before its kevent, and
wait_events (darwin) acquires ev() after its kevent, as the epoll path
does. 40 rounds of four concurrent runs, clean (it failed in 1 to 3
before); evloop is built with --race again.
