# An epoll registration outlives close while a child holds the file

Status: Observed (2026-10-12, linux/arm64, ThreadSanitizer), fixed in
std/llrt/xo_rt.c (pd_close, socket creation, accept, pipes).

Symptom: TestGoldenThreads/unix_kill (four mains at once in one process,
spawning processes and opening sockets) exited 66 with ThreadSanitizer
"use of an invalid mutex (e.g. uninitialized or destroyed)" in pd_ready,
called from ev_dispatch on the poller thread. Seen once in a full
internal/llvm run; not on the Mac.

Cause: two Linux behaviors together.
1. epoll keys a registration by the open file description, not by the
   descriptor number. close(fd) removes it only when no other descriptor
   refers to that file. The runtime never called EPOLL_CTL_DEL: it
   relied on close, then handed the xo_pd to the poller to free. If a
   child process still held the file, the registration stayed, and the
   next readiness event carried the freed xo_pd pointer.
2. Sockets, accepted connections, and pipes were made close-on-exec by
   fcntl just after creation. A process spawned by another thread in that
   window inherited the descriptor and held it for its whole life (which
   also keeps the connection open: the peer never sees EOF).
kqueue removes a descriptor's events when it is closed, so macOS never
showed it.

Fix: pd_close unregisters (EPOLL_CTL_DEL) when it marks the descriptor
closed, before shutdown and close; on Linux sockets are created with
SOCK_CLOEXEC, connections accepted with accept4(SOCK_CLOEXEC), and pipes
made with pipe2(O_CLOEXEC) (xo_pipe). Regression test: test_inherited in
std/llrt/testdata/evloop.c (TestRuntimeEventLoop): a pipe a child holds,
its xo_pd freed, then written; without the EPOLL_CTL_DEL it reports the
same ThreadSanitizer error on Linux (1 run of 1; with it, 2 runs of 2 pass).

Still open: macOS has neither accept4 nor pipe2, so the fcntl window
remains there (no crash, since kqueue drops the events, but a spawned
child can still inherit a socket briefly made by another thread).
