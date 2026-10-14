// rawpoll.c: the poller's epoll as Xo system calls (decision 0206 stage
// 3) under ThreadSanitizer, run by internal/llvm's TestRuntimeRawPoll on
// Linux. It includes the runtime, says the network goes through Xo (the
// net_raw export a program that dials carries), and registers descriptors
// from this thread while the poller thread waits: each registration's
// xo_pd is calloc'd here and locked by the poller when its event comes.
// The kernel orders the two; ThreadSanitizer sees that only through the
// release and acquire nx_epoll_ctl and nx_epoll_wait add around the
// system calls (gotchas/tsan-raw-epoll-no-edge.md). Without them every
// round is a "data race" report (pd_new's calloc against
// pthread_mutex_lock in pd_ready), as net_echo, http_wire, and net_only
// reported in TestGoldenTasks on Linux (2026-10-13).
//
// Exit status 0 means the test passed; it prints "rawpoll: ok".

#define XO_RT_NO_MAIN
#include "../xo_rt.c"

#if defined(__linux__) && !defined(XO_NOLIBC)
// the export a program that dials or listens carries (cabi_net.xo)
int64_t xo_x_net_raw(void) { return 1; }
#endif

int main(void) {
#if defined(__linux__)
	ev_start();
	// the poller is in epoll_wait before the first registration
	usleep(20000);
	int bad = 0;
	if (!nx_on()) {
		fputs("rawpoll: the network does not go through Xo\n", stderr);
		return 1;
	}
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
		pthread_mutex_lock(&pd->mu);
		int ready = pd->rready;
		pthread_mutex_unlock(&pd->mu);
		if (!ready) {
			usleep(50000);
			pthread_mutex_lock(&pd->mu);
			ready = pd->rready;
			pthread_mutex_unlock(&pd->mu);
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
