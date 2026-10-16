# Rust baselines for bench/perf (2026-10-16, xo-4)

Idiomatic Rust next to the hand written Go of two bench/perf suites:
the compute workloads of bench/perf/llvm (rust/: the same algorithms
and sizes as go/main.go, std only) and the concurrency workloads of
bench/perf/conc (rust/: tokio 1.47.1 for spawn, fanout, and pingpong,
as a Rust programmer writes lightweight tasks; std::thread::scope with
std's Mutex and RwLock for mutex, rwmutex, and pmap). Every Rust program
prints the Go version's checksum. No bench/perf suite has a hand written
C baseline (Mbed TLS, bench/perf/tls's C side, went on 2026-10-15), so
Go and Rust are the references.

Builds: Rust 1.99.1 (rustup stable, user-local on the Mac, the owner's
choice), `cargo build --release` with thin LTO, one codegen unit, and
panic=abort; Go 1.26.1; Xo built from main 024aea7c (this change adds
bench code and docs only), both native backends. Scripts:
bench/perf/llvm/rust.sh (the compute tables), bench/perf/conc/run.sh's
`rust` and `rust-ct` configurations, bench/perf/llvm/rust-docker.sh
(Linux instruction counts).

## Compute (bench/perf/llvm, Mac, arm64)

Best wall time of 5 interleaved runs, the fewest instructions retired
(`/usr/bin/time -l`), peak RSS. The Mac's load average was 15.7 at the
start (other sessions' builds): wall times are noisy, instruction counts
do not depend on load.

| Workload | go ms | rust ms | xo-llvm ms | xo-arm64 ms | go M instr | rust M instr | xo-llvm M instr | xo-arm64 M instr | xo-llvm/rust ms | xo-arm64/rust ms | go MB | rust MB | xo-llvm MB | xo-arm64 MB |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| fib | 235.9 | 174.6 | 233.8 | 228.4 | 5155 | 2996 | 3824 | 3327 | 1.34x | 1.31x | 4.4 | 1.9 | 2.2 | 2.4 |
| sieve | 119.4 | 113.4 | 104.8 | 105.5 | 2361 | 1703 | 2075 | 1707 | 0.92x | 0.93x | 47.7 | 56.4 | 18.1 | 18.3 |
| nbody | 169.1 | 119.3 | 192.0 | 180.6 | 3016 | 1410 | 2625 | 3610 | 1.61x | 1.51x | 4.4 | 1.9 | 2.2 | 2.4 |
| sort | 285.7 | 242.4 | 226.4 | 226.5 | 1854 | 1151 | 1314 | 1391 | 0.93x | 0.93x | 102.2 | 77.3 | 35.0 | 35.2 |
| interp | 148.2 | 51.9 | 70.3 | 48.7 | 1343 | 1293 | 1613 | 1431 | 1.35x | 0.94x | 4.4 | 1.9 | 2.2 | 2.4 |
| collatz | 525.7 | 350.3 | 500.2 | 500.3 | 3493 | 3048 | 3905 | 3616 | 1.43x | 1.43x | 4.5 | 1.9 | 2.2 | 2.4 |
| strings | 100.0 | 101.2 | 31.7 | 43.6 | 2937 | 2719 | 694 | 928 | 0.31x | 0.43x | 27.7 | 58.4 | 15.2 | 15.4 |

Instructions against Rust (the steadier measure): Xo's arm64 build is at
Rust on sieve (1.00x), 1.11x on fib and interp, 1.19x on collatz, 1.21x
on sort, 2.56x on nbody (the LLVM build 1.86x), and 0.34x on strings
(`acc = acc + w` appends in place; Rust's `format!` allocates and
formats each word). Go is 1.04x to 2.14x of Rust. Xo's peak memory is
the lowest on sieve, sort, and strings (18, 35, and 15 MB against
Rust's 56, 77, and 58 MB). Wall time: Xo's arm64 build is faster than
Rust on sieve, sort, interp, and strings (0.43x to 0.94x) and slower on
fib (1.31x), collatz (1.43x), and nbody (1.51x).

## Compute on Linux (instruction counts, linux/arm64 in Docker)

Exact instruction counts (valgrind 3.24's cachegrind with
--cache-sim=no, one run each; bench/perf/llvm/rust-docker.sh): Rust
built in rust:1.99.1-slim-trixie, Xo by a Linux xo built from main
024aea7c with the Go backend, run in scripts/docker-linux.sh's trixie
image.

| Workload | rust M instr | xo-llvm M instr | xo-arm64 M instr | xo-llvm/rust | xo-arm64/rust |
|---|---|---|---|---|---|
| fib | 2917.4 | 3808.5 | 3311.8 | 1.31x | 1.14x |
| sieve | 1856.6 | 2102.0 | 1683.4 | 1.13x | 0.91x |
| nbody | 1515.2 | 2535.1 | 3595.2 | 1.67x | 2.37x |
| sort | 1101.0 | 1292.5 | 1357.3 | 1.17x | 1.23x |
| interp | 1251.2 | 1566.1 | 1416.2 | 1.25x | 1.13x |
| collatz | 3031.6 | 3889.4 | 3600.2 | 1.28x | 1.19x |
| strings | 2490.6 | 681.1 | 927.7 | 0.27x | 0.37x |

Xo's arm64 build against Rust: 0.91x (sieve) to 1.23x (sort) on five
workloads, 2.37x on nbody, 0.37x on strings; the LLVM build 1.13x to
1.67x and 0.27x. The Mac's instruction ratios agree (fib 1.11x here
1.14x, nbody 2.56x and 2.37x).

Method notes: callgrind, the method of the earlier Linux rows, was
killed on the arm64 interp binary (its call graph grew past the Docker
VM's 8 GB; plain valgrind and cachegrind run it), so this table counts
with cachegrind. Where both ran, the counts agree within 0.5% (LLVM
interp 1,566.1M and 1,560.1M; Rust interp 1,251.2M and 1,245.2M), but
Rust's sieve came out 1,856.6M here and 1,716.6M under callgrind (an 8%
difference in a run dominated by Vec growth through glibc's realloc).

## Concurrency (bench/perf/conc, Mac, arm64)

Medians of 7 interleaved runs. `rust` is tokio's multi-threaded runtime
(the default), `rust-ct` the same binary on tokio's current-thread
runtime (CONCPERF_RT=current; mutex, rwmutex, and pmap use std threads
either way), `xo-arm64-1` Xo with XO_THREADS=1. The load average was
11.1 at the start.

Median wall ms of 7 runs

| Workload | go | rust | rust-ct | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 | xo-llvm/xo-go | xo-arm64/xo-go | xo-arm64-1/xo-go | xo-go/rust | xo-llvm/rust | xo-arm64/rust | xo-arm64-1/rust |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spawn | 67.1 | 46.7 | 18.2 | 231.7 | 74.8 | 75.6 | 49.7 | 0.32x | 0.33x | 0.21x | 4.97x | 1.60x | 1.62x | 1.06x |
| fanout | 36.0 | 38.4 | 10.8 | 115.3 | 31.1 | 32.2 | 25.7 | 0.27x | 0.28x | 0.22x | 3.01x | 0.81x | 0.84x | 0.67x |
| pingpong | 24.5 | 1134.1 | 15.5 | 65.9 | 33.2 | 34.7 | 30.9 | 0.50x | 0.53x | 0.47x | 0.06x | 0.03x | 0.03x | 0.03x |
| mutex | 22.0 | 7.3 | 7.3 | 138.6 | 10.4 | 13.4 | 11.8 | 0.08x | 0.10x | 0.09x | 19.00x | 1.43x | 1.83x | 1.62x |
| rwmutex | 12.6 | 17.1 | 17.7 | 402.8 | 100.8 | 105.4 | 41.7 | 0.25x | 0.26x | 0.10x | 23.56x | 5.90x | 6.17x | 2.44x |
| pmap | 63.4 | 54.0 | 53.4 | 82.3 | 67.3 | 68.9 | 277.2 | 0.82x | 0.84x | 3.37x | 1.53x | 1.25x | 1.28x | 5.14x |

Median CPU ms (user + sys) of 7 runs

| Workload | go | rust | rust-ct | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 | xo-llvm/xo-go | xo-arm64/xo-go | xo-arm64-1/xo-go | xo-go/rust | xo-llvm/rust | xo-arm64/rust | xo-arm64-1/rust |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spawn | 237.6 | 138.5 | 16.3 | 634.5 | 282.5 | 287.1 | 88.8 | 0.45x | 0.45x | 0.14x | 4.58x | 2.04x | 2.07x | 0.64x |
| fanout | 102.6 | 100.3 | 9.2 | 307.1 | 102.8 | 107.5 | 40.6 | 0.33x | 0.35x | 0.13x | 3.06x | 1.02x | 1.07x | 0.41x |
| pingpong | 33.4 | 1396.7 | 14.0 | 91.1 | 50.1 | 50.4 | 44.6 | 0.55x | 0.55x | 0.49x | 0.07x | 0.04x | 0.04x | 0.03x |
| mutex | 84.4 | 17.1 | 18.5 | 477.8 | 13.4 | 20.3 | 13.7 | 0.03x | 0.04x | 0.03x | 28.00x | 0.79x | 1.19x | 0.80x |
| rwmutex | 18.3 | 121.1 | 118.7 | 1703.8 | 192.5 | 204.7 | 47.9 | 0.11x | 0.12x | 0.03x | 14.07x | 1.59x | 1.69x | 0.40x |
| pmap | 564.8 | 466.0 | 464.2 | 735.9 | 612.3 | 615.0 | 542.1 | 0.83x | 0.84x | 0.74x | 1.58x | 1.31x | 1.32x | 1.16x |

Median Mcycles of 7 runs

| Workload | go | rust | rust-ct | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 | xo-llvm/xo-go | xo-arm64/xo-go | xo-arm64-1/xo-go | xo-go/rust | xo-llvm/rust | xo-arm64/rust | xo-arm64-1/rust |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spawn | 900.3 | 516.1 | 57.3 | 2396.2 | 1067.2 | 1084.2 | 337.2 | 0.45x | 0.45x | 0.14x | 4.64x | 2.07x | 2.10x | 0.65x |
| fanout | 386.2 | 373.0 | 29.8 | 1158.6 | 387.3 | 404.7 | 151.5 | 0.33x | 0.35x | 0.13x | 3.11x | 1.04x | 1.08x | 0.41x |
| pingpong | 121.8 | 5182.4 | 47.5 | 343.1 | 186.6 | 187.6 | 165.8 | 0.54x | 0.55x | 0.48x | 0.07x | 0.04x | 0.04x | 0.03x |
| mutex | 313.3 | 56.4 | 62.7 | 1818.8 | 45.3 | 71.1 | 47.2 | 0.02x | 0.04x | 0.03x | 32.24x | 0.80x | 1.26x | 0.84x |
| rwmutex | 64.0 | 415.3 | 399.6 | 6449.5 | 714.2 | 757.1 | 176.8 | 0.11x | 0.12x | 0.03x | 15.53x | 1.72x | 1.82x | 0.43x |
| pmap | 1959.3 | 1616.4 | 1612.5 | 2549.5 | 2114.2 | 2129.7 | 2104.0 | 0.83x | 0.84x | 0.83x | 1.58x | 1.31x | 1.32x | 1.30x |

Reading (Xo's arm64 build):

- Like for like on one thread (xo-arm64-1 against rust-ct): spawn 2.73x
  wall and 5.45x CPU, fanout 2.38x and 4.41x, pingpong 1.99x and 3.19x.
  Task creation, join, and channel hand-off cost Xo 2x to 5x what
  tokio's current-thread scheduler pays.
- Against tokio's multi-threaded default (rust): fanout 0.84x wall,
  spawn 1.62x, and pingpong 0.03x, where tokio takes 1,134 ms: every
  wake moves the other task to another worker. That row shows what the
  default idiom costs, not Rust's best (rust-ct: 15.5 ms).
- mutex: 1.83x wall, 1.19x CPU (std's futex Mutex); the LLVM build
  0.79x CPU.
- pmap: 1.28x wall, 1.32x CPU (64 OS threads against Xo's tasks on
  the pool).
- rwmutex: 6.17x wall and 1.69x CPU against Rust, 8.4x wall and 11.2x
  CPU against Go: the largest gap of either suite. Filed below.

## Concurrency after the scheduler work (2026-10-16, 13:44)

The same run (bench/perf/conc/run.sh, 7 interleaved rounds: every
workload in every configuration each round, so load hits them alike) on
main aca01d8a, after the scheduler changes of the day: the FIFO next
slot (68571558), deferred wakes with a monitor (e53d33a3) bounded by the
waker's next runtime call, with Mutex wakes direct (1ac5e830). The
machine was busy (four sessions): load average 17.4 at the start, 15.2
at the end (11.1 for the table above), so CPU and Mcycles are the
numbers to read, not wall time. Go's and Rust's own rows moved by up to
8% between the two runs (go spawn 900 to 838 Mcycles, fanout 386 to
397), which is the noise to allow.

Median wall ms of 7 runs

| Workload | go | rust | rust-ct | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 | xo-llvm/xo-go | xo-arm64/xo-go | xo-arm64-1/xo-go | xo-go/rust | xo-llvm/rust | xo-arm64/rust | xo-arm64-1/rust |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spawn | 65.9 | 46.9 | 18.1 | 232.0 | 74.0 | 72.2 | 52.3 | 0.32x | 0.31x | 0.23x | 4.95x | 1.58x | 1.54x | 1.12x |
| fanout | 36.2 | 39.6 | 10.8 | 108.3 | 32.6 | 33.8 | 26.6 | 0.30x | 0.31x | 0.25x | 2.73x | 0.82x | 0.85x | 0.67x |
| pingpong | 25.2 | 1138.4 | 15.6 | 63.3 | 26.2 | 27.2 | 31.4 | 0.41x | 0.43x | 0.50x | 0.06x | 0.02x | 0.02x | 0.03x |
| mutex | 23.6 | 7.2 | 6.8 | 142.1 | 10.8 | 12.3 | 8.9 | 0.08x | 0.09x | 0.06x | 19.82x | 1.51x | 1.71x | 1.25x |
| rwmutex | 11.7 | 16.9 | 17.0 | 389.0 | 93.6 | 93.4 | 42.8 | 0.24x | 0.24x | 0.11x | 23.00x | 5.53x | 5.52x | 2.53x |
| pmap | 65.0 | 53.1 | 54.8 | 83.7 | 70.4 | 71.4 | 279.8 | 0.84x | 0.85x | 3.34x | 1.58x | 1.33x | 1.35x | 5.27x |

Median CPU ms (user + sys) of 7 runs

| Workload | go | rust | rust-ct | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 | xo-llvm/xo-go | xo-arm64/xo-go | xo-arm64-1/xo-go | xo-go/rust | xo-llvm/rust | xo-arm64/rust | xo-arm64-1/rust |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spawn | 230.3 | 137.2 | 16.4 | 632.6 | 285.7 | 280.7 | 90.7 | 0.45x | 0.44x | 0.14x | 4.61x | 2.08x | 2.05x | 0.66x |
| fanout | 105.9 | 105.7 | 9.1 | 300.0 | 114.3 | 116.6 | 43.2 | 0.38x | 0.39x | 0.14x | 2.84x | 1.08x | 1.10x | 0.41x |
| pingpong | 34.4 | 1389.9 | 13.8 | 85.6 | 24.3 | 25.5 | 43.8 | 0.28x | 0.30x | 0.51x | 0.06x | 0.02x | 0.02x | 0.03x |
| mutex | 91.0 | 17.5 | 16.3 | 490.5 | 14.8 | 16.2 | 8.3 | 0.03x | 0.03x | 0.02x | 28.08x | 0.85x | 0.92x | 0.47x |
| rwmutex | 18.2 | 118.8 | 120.9 | 1633.2 | 156.8 | 156.8 | 49.1 | 0.10x | 0.10x | 0.03x | 13.75x | 1.32x | 1.32x | 0.41x |
| pmap | 558.5 | 455.4 | 451.6 | 727.2 | 615.4 | 627.2 | 547.1 | 0.85x | 0.86x | 0.75x | 1.60x | 1.35x | 1.38x | 1.20x |

Median Mcycles of 7 runs

| Workload | go | rust | rust-ct | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 | xo-llvm/xo-go | xo-arm64/xo-go | xo-arm64-1/xo-go | xo-go/rust | xo-llvm/rust | xo-arm64/rust | xo-arm64-1/rust |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spawn | 837.8 | 504.1 | 57.5 | 2387.7 | 1032.3 | 1005.4 | 275.6 | 0.43x | 0.42x | 0.12x | 4.74x | 2.05x | 1.99x | 0.55x |
| fanout | 396.9 | 387.7 | 29.2 | 1162.8 | 418.2 | 416.2 | 163.6 | 0.36x | 0.36x | 0.14x | 3.00x | 1.08x | 1.07x | 0.42x |
| pingpong | 125.9 | 5166.8 | 47.1 | 323.9 | 88.2 | 93.2 | 162.4 | 0.27x | 0.29x | 0.50x | 0.06x | 0.02x | 0.02x | 0.03x |
| mutex | 340.3 | 58.9 | 56.7 | 1837.7 | 48.8 | 54.5 | 26.0 | 0.03x | 0.03x | 0.01x | 31.19x | 0.83x | 0.93x | 0.44x |
| rwmutex | 62.6 | 412.3 | 423.1 | 6190.5 | 584.2 | 586.4 | 176.7 | 0.09x | 0.09x | 0.03x | 15.02x | 1.42x | 1.42x | 0.43x |
| pmap | 1964.2 | 1609.9 | 1602.2 | 2561.4 | 2150.4 | 2193.4 | 2117.4 | 0.84x | 0.86x | 0.83x | 1.59x | 1.34x | 1.36x | 1.32x |

xo-arm64 against the table above, Mcycles (CPU):

| Workload | before | after | after/before | xo-arm64/go after | xo-arm64/rust after |
|---|---|---|---|---|---|
| spawn | 1084.2 | 1005.4 | 0.93x | 1.20x | 1.99x |
| fanout | 404.7 | 416.2 | 1.03x | 1.05x | 1.07x |
| pingpong | 187.6 | 93.2 | 0.50x | 0.74x | 0.02x (rust-ct 1.98x) |
| mutex | 71.1 | 54.5 | 0.77x | 0.16x | 0.93x |
| rwmutex | 757.1 | 586.4 | 0.77x | 9.37x | 1.42x |
| pmap | 2129.7 | 2193.4 | 1.03x | 1.12x | 1.36x |

Reading:

- pingpong halves (0.50x cycles, 0.51x CPU ms): a channel hand off
  between two tasks now runs the woken task on the waker's worker
  (deferred wake) instead of waking a second worker per message. Xo's
  arm64 build now takes 0.74x Go's cycles and 0.74x its CPU; it was
  1.54x Go's cycles.
- mutex 0.77x and rwmutex 0.77x cycles: fewer idle worker wakes per
  hand off. rwmutex stays the largest gap (9.4x Go's cycles, 8.0x its
  wall time): a known gap, decision 0077's addendum (its cost is the
  migration of granted readers between workers, not the lock word or
  the futex calls).
- spawn 0.93x, fanout and pmap within noise (1.03x).
- On one worker (xo-arm64-1 against rust-ct, cycles): spawn 4.79x,
  fanout 5.60x, pingpong 3.45x (were 5.88x, 5.08x, 3.49x): task
  creation and join still cost Xo several times tokio's current-thread
  scheduler.

## rwmutex: contention, not work (filed)

Linux, callgrind (valgrind runs one thread at a time): 101.5M
instructions with XO_THREADS=1 and with 8, no parking at all; 57M of
it is the Collatz work inside the write lock. Mac, with real
concurrency (several threads run tasks even at XO_THREADS=1): 343M
instructions a run with one worker, 540M with 12, 4x Go's cycles.
`sample` of a 100x run with one worker: xo_stack_switch, mutex.wake,
pool.park_wait, park_signal, and tasks.park_wait_n against 79 samples
in the locked work: every contended rlock and lock parks (a fiber switch
and a ulock), every release wakes. XO_SPIN_ROUNDS=0 is faster than the
default 16 (1.39 s against 1.57 s at 20x), 64 slower (2.04 s). The fix
is in std/native/llrt/mutex.xo and pool.xo: spin on the RwLock's state
before parking, and grant readers without a switch when the granting
task is about to block. Next experiment: XO_THREADS=1 with a counter of
park_wait calls per lock operation in mutex.xo (rt.slot_add on a static
word), then a bounded spin in rlock before wl_add.

## Go under valgrind

Go 1.26.1 binaries for linux/arm64 crash under valgrind 3.24 at start
(SIGSEGV in runtime.getCPUCount, os_linux.go:101), static or with cgo,
at any --main-stacksize, so the Linux table has no Go column.
