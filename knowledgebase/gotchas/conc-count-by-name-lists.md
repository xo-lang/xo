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
both flags. A std module that waits on something else outside these
lists (a file, a timer of its own) would be missed the same way: give
its waiting functions an effect and count it, or add them to the lists.

Check: a golden of the module with no task in the program
(dns_no_tasks, udp_no_tasks), and for the shutdown, internal/llvm
TestNativeSignalNetNoTasks (SIGTERM twice to its own child).
