# The compiler's concurrency count is by name lists, and std modules can miss it

Status: Observed (2026-11-03, Xo Lang 1), fixed for the network

Symptom: a program with no task in its code whose only network was
std/udp did not build (`_xo_x_nc_conn_free` is named in
`netconn.conn_new` but no function of the program defines it); one whose
only network was std/dns built, but ran without the scheduler: SIGTERM
killed it at once (status 143) with no defer run and its buffered output
lost, and a netpoller wait in it waited for the poller thread's tick
(gotchas/netpoll-wait-idle-wake-latency.md). The same calls in a program
with any spawn in its code, even one never run, behaved.

Cause: whether a program keeps the scheduler, the poller and the monitor
(concflag.xo, the ADR 0371 addendum) is decided by the SSA builder at the
call sites it lowers: scopes, tasks, channels, Mutex, Atomic, the
prelude's Net, Listener and Conn methods (ssa/call.xo's table),
native_waits (processes), sleeps and standard input reads. std/dns and
std/udp call native/llrt/netconn directly from their native modules, so
none of those names appeared; uses_net (which builds cabi_net's exports)
was missed the same way.

Fix: ssa/call.xo net_waits: a call of a std function (not the prelude's)
whose declared effects include `net` or `net.*` (check.declares_net) sets
both flags (b8a051ecf). Then the class (2026-11-03, compiler/ssa/waits.xo):
every std function declaring a network effect, the prelude's Net,
Listener and Conn methods too, counts as waiting from its effect, so a
new network module needs no entry. The other effects are too coarse to
decide it (`clock` is now and sleep, `stdio` print and read, `proc` exit
and spawn, `fs` a read and a lock), so their waiting methods are the
WAITS list and the rest NO_WAIT, and internal/repotest
TestWaitsClassified fails when a std function with no Xo body declares
clock, stdio, proc or fs and is in neither list (or a listed name is
gone). A new std module that waits on something else must declare one
of those effects, and the test then asks for its entry.

Check: a golden of the module with no task in the program
(dns_no_tasks, udp_no_tasks), and for the shutdown, internal/llvm
TestNativeSignalNetNoTasks (SIGTERM twice to its own child).
