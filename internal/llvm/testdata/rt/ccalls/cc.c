// C calls and the scheduler (decision 0675, task #94).
#include <errno.h>
#include <stdatomic.h>
#include <time.h>
#include "cc.h"

static atomic_long flag;

static int64_t now_ms(void) {
	struct timespec t;
	clock_gettime(CLOCK_MONOTONIC, &t);
	return (int64_t)t.tv_sec * 1000 + t.tv_nsec / 1000000;
}

/* nanosleep(ms) as SQLite's unixSleep calls it: 1 when it returned early
   (EINTR), else 0 */
int64_t cc_sleep(int64_t ms) {
	struct timespec sp = { ms / 1000, (long)(ms % 1000) * 1000000L };
	return nanosleep(&sp, 0) != 0 && errno == EINTR ? 1 : 0;
}

/* waits in C up to ms for cc_set: 1 when it came, 0 on the timeout */
int64_t cc_wait(int64_t ms) {
	int64_t end = now_ms() + ms;
	while (atomic_load(&flag) == 0) {
		if (now_ms() >= end) {
			return 0;
		}
		struct timespec sp = { 0, 1000000L };
		nanosleep(&sp, 0);
	}
	return 1;
}

void cc_set(void) { atomic_store(&flag, 1); }
