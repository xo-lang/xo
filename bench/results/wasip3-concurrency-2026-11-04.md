# Decision 0447 stage 5 step 3: several requests per instance at WASI 0.3 against Rust (2026-11-04)

First linux/arm64 offload host, alone on it (an exclusive lease),
wasmtime 50.0.1 for every row with `wasmtime serve`'s defaults for a
0.3 component: up to 16 requests at once per instance, 128 per instance
in all. Each component serves:

- `GET /`: answers `hello`.
- `GET /proxy`: sends one GET to a Go server on 127.0.0.1:18999 that
  answers `upstream` after 5 ms, and answers with its body.

How each side was built:

- Xo before (step 2, one request at a time per instance: the export
  set `backpressure`) and Xo after (step 3): the same program, built for
  `--targets=wasip3/wasm` by xo at fe7a71942 (main) and at 4e7849f5e (branch l4-s5c).
- Rust 0.3: the `wasip3` crate 0.10.0 with wit-bindgen 0.63.0, built as
  in [wasip3-stream-client-2026-11-04.md](wasip3-stream-client-2026-11-04.md).
- Load: loadgen3 -c 32 and -c 256, 15 s per route and level, each under
  a fresh `wasmtime serve`; memory is that process's peak resident size
  (VmHWM) at the end of the run, which covers every instance it made.
  Three rounds, the three components in turn each round.

Medians of the three rounds:

| | bytes | req/s c32 | p99 c32 | req/s c256 | p99 c256 | peak RSS c32 | peak RSS c256 |
|---|---|---|---|---|---|---|---|
| hello, Xo before | 516,371 | 121,329 | 1.025 ms | 206,789 | 5.312 ms | 103,932 KB | 183,936 KB |
| hello, Xo after | 550,870 | 115,973 | 1.054 ms | 202,599 | 5.435 ms | 115,972 KB | 189,892 KB |
| hello, Rust 0.3 | 117,271 | 130,818 | 1.017 ms | 207,995 | 5.103 ms | 116,804 KB | 152,700 KB |
| proxy, Xo before | 516,371 | 4,784 | 9.198 ms | 14,466 | 82.370 ms | 751,872 KB | 5,454,548 KB |
| proxy, Xo after | 550,870 | 4,915 | 8.649 ms | 14,440 | 81.851 ms | 507,468 KB | 2,355,312 KB |
| proxy, Rust 0.3 | 117,271 | 4,963 | 8.377 ms | 14,513 | 77.921 ms | 66,952 KB | 146,376 KB |

Reading:

- req/s and p99 match Rust at both levels, before and after: one
  request per instance already kept up, since wasmtime started an
  instance per request in flight. Hello is 4% under Rust's req/s at 32
  connections after the change (115,973 against 121,329 before).
- Memory is the gap. Several requests per instance cut the proxy's peak
  by a third at 32 connections and by 57% at 256, but it stays 7.6x and
  16x Rust's. A debug build found the cause: the wasm backend's
  memory-frame stack, one 16 MB block resident in every instance that
  reaches a function with more than 160 locals, which the client path
  does (gotchas/wasm-frame-stack-resident.md). A 1 MB stack measured
  167 MB and 322 MB on the second host; the fix (a stack in chunks) is
  measured separately.
- Before the fix of its EXIT test, the new component stopped at exactly
  42,623 requests in every run (333 instances of 128 requests each, then
  wasmtime's cap of 1,000 core instances): a finished request never
  returned EXIT, so no instance was torn down.

Raw output: the lead's notes, xo-lang-4/out/s5m3-2.txt.

## With the frame stack in chunks (c2ae2814c)

Same host, program and method (3 rounds, medians), xo at 79a8d6554 (step 3
alone) against c2ae2814c (step 3 plus the memory-frame stack in chunks,
gotchas/wasm-frame-stack-resident.md), with Rust's component:

| proxy | req/s c32 | p99 c32 | req/s c256 | p99 c256 | RSS c32 | RSS c256 |
|---|---|---|---|---|---|---|
| Xo, step 3 | 4,919 | 8.689 ms | 14,413 | 78.817 ms | 485,432 KB | 2,570,892 KB |
| Xo, chunked stack | 4,925 | 8.366 ms | 14,414 | 76.251 ms | 74,344 KB | 169,064 KB |
| Rust | 4,968 | 8.344 ms | 14,473 | 83.286 ms | 67,976 KB | 152,692 KB |

| hello | req/s c32 | p99 c32 | req/s c256 | p99 c256 | RSS c32 | RSS c256 |
|---|---|---|---|---|---|---|
| Xo, step 3 | 116,466 | 1.040 ms | 201,366 | 5.495 ms | 111,164 KB | 181,696 KB |
| Xo, chunked stack | 115,795 | 1.039 ms | 203,813 | 5.373 ms | 110,816 KB | 191,712 KB |
| Rust | 131,981 | 1.012 ms | 204,617 | 5.255 ms | 110,052 KB | 163,756 KB |

Reading:

- The proxy's peak memory is 1.09x Rust's at 32 connections and 1.11x at
  256 (was 7.1x and 16.8x); req/s and p99 are unchanged and within Rust's
  run to run spread.
- hello does not reach a framed function, so it is unchanged; its peak at
  256 connections is 1.17x Rust's (round 1's 167 to 177 MB at c32, for
  both Xo builds, is the first run's warm up).
- Component sizes: 554,388 and 549,857 bytes against Rust's 117,271 (the
  next item, component size).

Raw output: the lead's notes, xo-lang-4/out/mm-1.txt.
