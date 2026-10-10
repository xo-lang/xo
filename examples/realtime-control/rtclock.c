#include <time.h>

#include "rtclock.h"

// The monotonic clock in nanoseconds. macOS's CLOCK_MONOTONIC counts in
// microseconds, so it reads the uptime counter (about 42 ns ticks on
// Apple silicon) instead.
long long rt_now_ns(void) {
#ifdef __APPLE__
	return (long long)clock_gettime_nsec_np(CLOCK_UPTIME_RAW);
#else
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (long long)ts.tv_sec * 1000000000LL + ts.tv_nsec;
#endif
}
