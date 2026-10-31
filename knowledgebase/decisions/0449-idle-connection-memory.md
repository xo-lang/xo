# 0449: idle connections cost no stack and no mapping

Status: Accepted (owner, 2026-10-31, as written: parking, no shared stack
mappings, the h2 heap cut; Xo Lang 3, 2026-10-31). Triggered by Go 1.28's idea of parking idle
HTTP/2 connections; related to 0069 (fibers), 0301 (workers poll) and
0395.

## Context

Measured 2026-10-31 on the linux/arm64 offload host (Go 1.26.1, 12 CPUs,
bench/perf/idleconn: a client opens N connections, completes one request
on each, then holds them idle; the server's RssAnon and `/proc/PID/smaps`
are read). Per idle connection, server RssAnon:

| connections | mode | Xo | Go |
|---|---|---|---|
| 1,000 | h1 | 38 KB | 27 KB |
| 5,000 | h1 | 17 KB | 21 KB |
| 1,000 | h2/TLS | 127 KB | 53 KB |
| 5,000 | h2/TLS | 91 KB | 44 KB |

h1 is on par with Go. h2 is about twice Go. The smaps breakdown at
5,000 h2 connections:

- 10,323 mappings (Go: 108). Each connection owns two: its fiber's stack
  (pool.xo map_stack: 64 MiB reserved, MAP_NORESERVE, the bottom page
  mprotected PROT_NONE as a guard) is a guard VMA and a stack VMA.
- The stacks' resident memory: 161.6 MB, 32 KB per idle h2 connection
  (h1: 5.6 KB).
- The heap: 281.6 MB in the 1 to 16 MB class, 56 KB per idle h2
  connection (h2 connection state, the TLS and h2 buffers, the HPACK
  tables). Go's whole 44 KB per connection is one heap, its goroutine
  stack included.

Two limits follow. First, Linux's `vm.max_map_count` is 65,530 by
default: an Xo server cannot hold more than about 32,000 idle
connections (at the limit the next mmap fails: "xo: fiber stack mmap
failed", cabi.xo). Go has no such limit, since a goroutine stack is
heap. Second, memory: at 100,000 idle h2 connections Xo needs about 9 GB
against Go's 4.4 GB.

## Decision (proposed)

Three parts, each measured before the next (the 40,000 connection test
below, and RssAnon per connection from bench/perf/idleconn).

1. **Idle connections park.** A connection waiting for its next bytes
   is a registration in the poller (the descriptor, a small record with
   the protocol state), not a task on a fiber stack. When bytes arrive
   the poller starts a task on a stack from the cache, which reads,
   handles what arrived and parks again; a request that finishes with
   nothing buffered returns its stack. Stacks in use then scale with
   active requests, not with connections. This is what Go 1.28 does for
   HTTP/2 (parking the connection's reader), and what 0301's poller
   threads already prepare: the poller owns the readiness, the pool owns
   the CPU.
   - Stage 1: the server loops in std/http (h1 keep-alive wait, h2
     read loop, the TLS record read) wait through a `park_read` that
     leaves the fiber (the stack goes back to the cache) when the
     descriptor has no bytes; the reader restarts on a fresh stack. No
     API change.
   - Stage 2: WebSocket and other long lived readers use the same call.
2. **Not chosen: fiber stacks sharing mappings.** A pooled stack arena
   (one large mapping carved into stacks) keeps the stack count equal
   to the connection count and the guard pages are still separate
   PROT_NONE VMAs: [guard][stack][guard][stack] is two VMAs per stack
   whatever the mapping that holds them. Removing the guards removes
   the stack overflow fault (crash.xo on_overflow) that turns deep
   recursion into a clear error. An arena would fix address space use,
   not the VMA count, and would not touch the 32 KB resident stack.
3. **The h2 heap cut.** Allocate an h2 connection's buffers on use:
   the read and write buffers at the first frame instead of at accept,
   the HPACK dynamic table at its size on use (4,096 bytes of limit is
   not 4,096 bytes of allocation), the per-stream map empty until a
   stream opens, TLS record buffers released when the connection is
   idle. Target: at most Go's 44 KB per idle h2 connection in total.

## Test that fails today

`TestIdleConnections40k` (Linux only, `-run` through the offload host,
`ulimit -n` of at least 100,000): the idleconn server and client with
40,000 h1 connections held idle for 10 s, asserting that every
connection stays usable (a request on each succeeds) and that the
server's `/proc/PID/maps` line count stays under 20,000. Expected to fail
today: at about 32,700 connections the stack mmap should fail and the server exits
(or refuses the connection): predicted from the two mappings per
connection and the default limit, not yet run (the first step of
stage 1 is to run it and record the result here). A second assertion (RssAnon per idle h2
connection at 5,000 connections at most 60 KB) fails today at 91 KB and
guards part 3.

## Stage 1 design: park_read in the h1 keep-alive wait (Xo Lang 3, 2026-10-31)

Where the stack is held: `serve_conn` (std/native/http.xo) is one task per
connection; after a response it loops to `rd.read()`, which waits in
`poll.pd_wait` on the connection's descriptor with the whole fiber stack
live. Everything the loop needs between two requests is in heap values
(`hc`, the reader `rd` with its buffered bytes, `r`, `lim`); nothing is
only on the stack at that point.

The cut:

1. `poll.pd_park(pd, resume)`: the descriptor's wait record gets a third
   state beside D_RWAIT (a parked task) and READY: D_CONT, a heap closure
   `resume: fn()` (an `rt.Any` held with a reference). If the descriptor
   is already readable it returns false and the caller reads on. If not,
   it stores the continuation, arms the idle timer (below) and returns
   true.
2. `serve_conn`'s loop, when `rd` has no buffered bytes after a response:
   `if hserve.hc_park(hc, fn() { serve_conn_resume(hc, r, serve_h2, lim, rd) }) { parked = true; return }`.
   The `defer hserve.hc_close(hc)` becomes `defer { if !parked { hc_close(hc) } }`.
   `serve_conn_resume` is the `while true` loop; `serve_conn` calls it
   after its setup. The task ends and its stack goes back to the cache.
3. The poller (`pd_ready`) finding D_CONT with no D_RWAIT waiter spawns a
   task running `resume` in the server's scope (`hserve.srv_group`), so
   `srv_wait` still waits for it; a parked connection counts as live in
   the server's list (`SR_CONNS`) but not as a task.
4. Stop and close: `srv_stop` closes every parked connection (net/http's
   Shutdown closes idle connections), by walking the connection list and
   taking each parked continuation (compare-and-swap D_CONT to 0, so a
   concurrent readiness resume and the stop do not both run it); `pd`
   close takes the same path, and the resumed task finds the closed
   descriptor on its first read and ends the connection.
5. The idle timeout (`lim.read_ns` / net/http's IdleTimeout) is the pd's
   deadline timer (`pd_timer`): its expiry takes D_CONT the same way and
   resumes the closure, whose read then reports the timeout.
6. TLS (stage 1b): the same, with the wait placed where the TLS record
   read blocks (`tlssrv` record read), because the TLS state is in the
   connection record already. HTTP/2 (stage 1c): the read loop parks the
   same way when no stream task is running and no frame is buffered; a
   connection with streams in flight keeps its reader.

Mechanics found reading the code: the descriptor record (layout.xo D_*)
has no free word (D_MU 0, fd 8, closed 12, ops 16, ready flags, D_RWAIT
32, D_WWAIT 40, D_NEXT 48, D_TIMERS 56, D_SIZE 64; the Windows words start
at 64), so D_CONT is word 64, D_SIZE 72 and the DW_* words move up 8.
poll.xo cannot import tasks.xo (tasks imports poll), so `pd_ready` fires a
continuation through a function table entry like `poll_park_wait`
(`rt.call_c2(rt.fn_addr("pd_cont_fire"), pd, cont)`), defined in
hserve.xo, which imports both and calls `tasks.spawn` with the stored
closure words; `spawn` already runs on a thread that is not a worker
(`poolsetup.root_worker`), as the poller is.

As built (first cut, 2026-10-31, branch park1): the poller does not spawn.
`pd_ready` and `pd_close` hand a fired continuation record to its
server's ready list (`hserve.cont_fire`, reached through an address
stored in the event record, since a program without a server has no
`pd_cont_fire` export); one resumer task per server (`hserve.resume_next`,
spawned beside the accept loop in `Server.serve`) takes the list and
starts each resume function in the connection scope. The park point is
`serve_h1`'s top of loop: no buffered request bytes, no server idle
timeout (`with_idle_timeout` connections keep their stack; a pd deadline
timer that resumes the continuation is stage 1d), a plain TCP connection
(`hconn.park_ready`: a `MSG_PEEK` finds the socket empty), the server not
stopping. TLS (1b) needs a "plaintext pending" operation on the TLS
engine before a raw peek can say a connection is empty; HTTP/2 (1c)
needs the stream-free state. The reader is rebuilt on resume
(`http1.reader(hc)`): its state lives in the connection's X record.

Risks to measure first: the 32 connection CRUD server (bench/perf/http)
parks rarely, so `xo bench --ab` must show no slowdown there; a parked
connection pays one task start per request (a stack from the cache, no
mapping). Cancellation: a task canceled while a connection is parked has
nothing to cancel; the stop path above covers shutdown.

Proof: `TestIdleConnections40k` (internal/stdtest/idleconn_test.go,
`XO_TEST_IDLE40K=1`, linux): 40,000 idle h1 connections, each answers a
second request, fewer than 20,000 mappings in the server.

## Consequences

- Fiber stacks stop being the per-connection cost; a server with many
  idle connections and few active requests uses memory like Go's.
- A parked connection's per request work grows by a task start (a stack
  from the cache, not a mapping): to be measured with `xo bench --ab`
  on the CRUD server of bench/perf/http (32 connections, mostly active),
  where connections rarely park. The gate for stage 1 is no slowdown
  there beyond noise.
- Part 1 touches how a blocked read ends a fiber, so stack addresses
  captured across a read are invalid after it (the conservative scan and
  the stack traces already treat a fiber's stack as dying at its end).
- Part 2 is recorded so it is not re-proposed: the guard page is the
  cost.

## Alternatives

- Raise `vm.max_map_count` in deployment docs: a real workaround that
  hides the 32 KB stack and the 2x memory; documented as such, not
  chosen.
- Smaller stacks (XO_TASK_STACK_MB): the reservation is virtual and the
  VMA count unchanged; helps neither limit.
- Guard-less stacks with stack probes: removes a VMA per stack but also
  the overflow fault; not chosen with part 1 available.
