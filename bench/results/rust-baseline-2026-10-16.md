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
