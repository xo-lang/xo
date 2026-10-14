# A C thread local's address costs a call on macOS

Status: Observed (2026-10-14, work item B01, Clock.now on macOS)

Symptom: Clock.now from Xo on macOS took 1.051 (arm64) and 1.072 (LLVM)
times the C path's instructions in bench/perf/sysio's clock workload,
although its own path (a load of the clock word, a compare,
clock_gettime_nsec_np) was shorter than the C path's. The extra cost
was in rt.sys_clock_buf: std/llrt/xo_sys.c xo_sys_clock_buf answered
the address of the `__thread` scratch buffer on every OS.

Cause: Mach-O thread locals are reached through a TLV descriptor whose
getter is a function call (`tlv_get_addr`): dropping the access saved
about 16 instructions per Clock.now on arm64 (102.5 to 94.5 million
over 500,000 calls); on Linux (local exec in a static program) and Windows the
address is a register plus an offset.

Fix: keep thread local addresses out of macOS hot paths that do not
need them. xo_sys_clock_buf answers 1 on macOS (libSystem's clock takes
no structure), keeping only the virtual clock test: 0.969 and 0.977
(bench/results/clocks-2026-10-14.md). Where Xo needs per thread
scratch, rt.tlocal_addr's runtime table (emitted by the compiler) is
the cheap route on Linux; check macOS separately.
