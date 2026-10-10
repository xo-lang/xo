// rtclock.h: the monotonic clock in nanoseconds, for the real-time demo
// (examples/realtime-control, decision 0133). Bound with `use c ... uses
// none`: reading the clock has no effect a real-time function must not
// have.
long long rt_now_ns(void);
