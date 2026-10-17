# Lazy runtime bodies (decision 0247), 2026-10-16

xo-4b, the development Mac (M4 Pro), the compiler of branch play-driver
built from main 02792d0c plus decision 0247, `XO_NO_SERVE=1
XO_NO_CACHE=1`; eager is the same compiler with `XO_CHECK_EAGER=1`.
Wall times, the machine shared with other sessions' runs.

## Same output

| What | Compared | Differences |
|---|---|---|
| every golden (internal/llvm/testdata/golden), arm64: the binary and the build's output | 180 | 0 |
| every golden, LLVM: the IR (`xo asm --backend=llvm`) and output | 180 | 0 |
| selfhost/xo, arm64: the binary | 1 | 0 |

LLVM binaries were compared by IR: two eager LLVM builds of one golden
already differ (at byte 1529 of arith's binary), arm64's do not. A build
names its output in its first line, so the outputs were built under one
name in two directories.

## Build times

| Build | Eager | Lazy | Change |
|---|---|---|---|
| hello (`fn main` printing a sorted list), arm64, best of 5 | 947 ms | 622 ms | -34% |
| all 180 goldens, arm64, one build each, summed | 289.6 s | 227.6 s | -21% |
| selfhost/xo, arm64 (second runs; the first lazy run was cold, 33.90 s) | 15.31 s, 15.74 s | 16.03 s | none |

A small program reaches a small part of the runtime, so most of its
runtime bodies are no longer checked. The compiler reaches most of it,
and its own modules are checked in full either way.

## Where the time went before (profiles)

- `xo build --backend=arm64` of hello (XO_PROF=cpu, 935 samples at
  1000 Hz): check_program 47.9%, of it check_module (bodies) 24.1% and
  parsing 14.7%; the SSA builder 7.4%.
- The same build by the wasm build of selfhost/wasm under Node
  (`--cpu-prof`, function names from the annotated .wat; busy time
  16.9 s under the profiler, 8.5 s without): check_module 9.6 s (61%),
  flow_bodies 1.8 s (11%), parse_into 1.9 s; the SSA builder's queue
  0.1 s.
