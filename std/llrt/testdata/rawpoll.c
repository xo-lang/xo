// rawpoll.c: the poller's epoll as Xo system calls (decision 0206 stage
// 3) under ThreadSanitizer, run by internal/llvm's TestRuntimeRawPoll on
// Linux. It includes the runtime and registers descriptors from this
// thread while the poller thread waits: each registration's xo_pd is
// allocated here (poll.xo pd_open) and locked by the poller when its
// event comes. The kernel orders the two; ThreadSanitizer sees that only
// through the release and acquire poll.xo adds around the system calls
// (epoll_add, wait_events; gotchas/tsan-raw-epoll-no-edge.md). Without
// them every round is a "data race" report (the registration's writes
// against the lock in pd_ready), as net_echo, http_wire, and net_only
// reported in TestGoldenTasks on Linux (2026-10-13). UNTESTED on linux
// since the event loop's start moved to poll.xo (2026-10-15).
//
// Exit status 0 means the test passed; it prints "rawpoll: ok".

#include "../xo_rt.c"
#include "xcalls.h"
#include <unistd.h> // pipe, write, close, usleep (xo_rt.c no longer brings it)

// pd's read readiness under its lock (poll.xo D_MU at 0, D_RREADY at 20)
static int pd_rready(xo_pd *pd) {
	lk((xo_lock *)pd);
	int r = *(int *)((char *)pd + 20);
	unlk((xo_lock *)pd);
	return r;
}

// pd_new registers fd with the poller (poll.xo pd_open, which xo_rt.c no
// longer wraps): NULL when the poller refuses it.
int64_t xo_x_poll_pd_open(int64_t fd, int64_t write);
static xo_pd *pd_new(int fd, int write) {
	int64_t r = xo_x_poll_pd_open(fd, write);
	return r < 0 ? NULL : (xo_pd *)(intptr_t)r;
}

int main(void) {
#if defined(__linux__)
	ev_start();
	// the poller is in epoll_wait before the first registration
	usleep(20000);
	int bad = 0;
	for (int i = 0; i < 20; i++) {
		int p[2];
		if (pipe(p) != 0) {
			perror("pipe");
			return 1;
		}
		set_nonblock(p[0]);
		xo_pd *pd = pd_new(p[0], 0);
		if (!pd) {
			bad++;
			close(p[0]);
			close(p[1]);
			continue;
		}
		// readable: the poller's event names the xo_pd, and pd_ready locks it
		if (write(p[1], "x", 1) != 1) {
			bad++;
		}
		usleep(5000);
		int ready = pd_rready(pd);
		if (!ready) {
			usleep(50000);
			ready = pd_rready(pd);
		}
		bad += !ready;
		pd_free(pd);
		close(p[1]);
	}
	// the poller frees the descriptors at the top of its next turn
	ev_poke();
	usleep(20000);
	if (bad) {
		fprintf(stderr, "rawpoll: %d of 20 registrations saw no event\n", bad);
		return 1;
	}
	fputs("rawpoll: ok\n", stderr);
#else
	fputs("rawpoll: ok (epoll is Linux only)\n", stderr);
#endif
	return 0;
}
