# ThreadSanitizer saw no edge through the poller's raw epoll calls

Status: Observed, fixed (2026-10-13, netfix agent; linux/arm64 and
linux/amd64 reports, fix verified on linux/arm64).

Symptom: on Linux, internal/llvm's ThreadSanitizer builds of net_echo,
http_wire, and net_only (TestGoldenThreads, now TestGoldenTasks) report
a data race: `pthread_mutex_lock` in pd_ready (ev_dispatch, the poller
thread) against the `calloc` in pd_new (xo_net_dial_timeout on the
dialing thread), on the 88 byte xo_pd. Seen on c7i.8xlarge (amd64) and
r8g (arm64), at main 4f792c55 and later; never on the Mac (kqueue goes
through the C library).

Cause: a false positive. pd_new allocates and initializes the xo_pd
fully, then registers it with epoll_ctl (its pointer in the event's
data); the poller gets that pointer back from epoll_wait and only then
locks it. The kernel orders the two, and the C library's epoll_ctl and
epoll_wait interceptors tell ThreadSanitizer so (epoll_ctl releases the
epoll descriptor, epoll_wait acquires it when events return). Since
decision 0206 stage 3 a program that dials or listens makes those calls
as Xo system calls (cabi_poll.xo through nx_epoll_ctl and
nx_epoll_wait), which no interceptor sees, so the edge disappeared.
Evidence (bench/results/task-switch-2026-10-13.md, "Linux network
fixes"): without the fix the C test races and net_echo races in every
run; with `XO_LLRT_C=net` (the C library's epoll, intercepted) the same
code without the fix is clean.

Fix: nx_epoll_ctl calls `__tsan_release` on a static sync object
(nx_poll_sync) before the system call, and nx_epoll_wait calls
`__tsan_acquire` on it when events return, the same pair the
interceptors make (std/llrt/xo_rt.c, TSAN_RELEASE and TSAN_ACQUIRE, no
code in other builds). TestRuntimeRawPoll (std/llrt/testdata/rawpoll.c)
registers pipes from the main thread with the network forced through
Xo and fails on Linux without the pair.

Update (2026-10-15, the zero-C audit's B22 remnants): when the poller's
loop moved to std/native/llrt/poll.xo, the acquire half went missing
(only the C registration's release stayed) and rawpoll.c went stale
(nx_on, pthread_mutex_lock on what is now a lock word). Both sides are
poll.xo's now, on the ev record as the sync object: epoll_add releases
it before EPOLL_CTL_ADD and wait_events acquires it after epoll_wait
(rt.sched_rt 22 and 23); rawpoll.c is updated, UNTESTED on linux. A move
of a C loop into Xo must carry its TSan annotations over too.

Avoid: any C runtime hand off that a raw system call replaces (a pipe
write the poller reads, a futex, an eventfd) loses the interceptor's
edge the same way; add the pair where the C library call used to be
(see also tsan-blind-to-xo-synchronization.md).
