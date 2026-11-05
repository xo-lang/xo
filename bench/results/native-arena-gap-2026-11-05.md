# What a native request arena could save (2026-11-05)

For decision 0524 (Proposed). Main 56e08ee4b, linux/arm64 offload host 1.

## Throughput (bench/perf/http run.sh, REPS=3, 32 workers, 10 s after 2 s warmup; medians)

| Config | Req/s | p50 us | p99 us | Peak RSS MB | CPU us/req |
|---|---:|---:|---:|---:|---:|
| go | 125,138 | 186 | 1,148 | 19.1 | 27.17 |
| xo native arm64 | 190,552 | 134 | 847 | 13.1 | 11.54 |
| xo native LLVM | 189,114 | 135 | 842 | 12.8 | 11.82 |

run.sh's `xo` (--no-arenas) and `xo-arenas` configs now build with the default (native)
backend, which ignores arenas: their rows (188,772 and 189,568 req/s) match the native
ones, so the Go backend's arena gain (decision 0066) is no longer in this table.

## Where the native server's CPU goes

srv-xo-arm64 under the same load with XO_PROF=cpu XO_PROF_HZ=2000 (190,069 req/s, 12.1 us
CPU per request with the profiler): 32,768 samples kept and 21,500 lost to the sample
table, so the shares are indicative. `go tool pprof -top -nodefraction=0`:

| Where | Flat ms | Share |
|---|---:|---:|
| hconn.send_at (write syscall) | 4,057 | 24.8% |
| ? (under sysio.mono_ns) | 1,844 | 11.3% |
| sock.read | 1,496 | 9.1% |
| pool.futex_wake | 864 | 5.3% |
| poll.wait_events | 791 | 4.8% |
| xo_stack_switch | 621 | 3.8% |
| allocator and RC (every alloc., drop and inc node) | 1,438 | 8.8% |
| total | 16,384 | |

The allocator and RC rows: drop Str 268 ms, inc thunk_x9 189, drop List[Str] 150,
alloc.free_push 150, drop thunk Str_body 146, alloc.alloc 110, then a tail of nodes under
60 ms each. sysio.mono_ns (11.5% cumulative) is called 79% from pool.find_work, the idle
workers' spin, so it is waiting, not request work (the clocksource is arch_sys_counter).

An arena saves at most the 8.8%, less its own bump allocation, reset and debug check.

Scripts: ~/xo-lead/notes/xo-lang-4/scripts/arena-remeasure-remote.sh,
arena-prof-remote.sh, arena-prof-sum2-remote.sh.
