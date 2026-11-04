# Decision 0447 stage 5 step 1: hello at WASI 0.3 against 0.2 and Rust (2026-11-03)

Second linux/arm64 offload host (18 cpus), alone on it (an exclusive
lease), wasmtime 50.0.1 for every row. All three components serve `GET /`
with the 5 byte body `hello`.

- Xo 0.2: `xo build --targets=wasip2/wasm` of the stage 2 hello program
  (decision 0447's Stage 2 results), at commit 13a3cab21 of branch l4-s5.
- Xo 0.3: the same program and xo, `--targets=wasip3/wasm`.
- Rust 0.3: the `wasip3` crate 0.10.0's http-proxy example reduced to
  hello, wit-bindgen 0.63.0 (async-spawn), `cargo build --release
  --target wasm32-wasip2`, `opt-level = "s"`, LTO, stripped.
- Each row: size of the built file; first request = `wasmtime serve
  -Scli` start to the first 200 (curl every 5 ms), median of 5; then
  loadgen -c 32 -d 20s, which fails on any body but `hello`. Three
  rounds, the three components in turn each round.

Medians of the three rounds:

| component | bytes | first request | req/s | p50 | p90 | p99 |
|---|---|---|---|---|---|---|
| Xo 0.2 (wasip2) | 322,996 | 31.3 ms | 95,114 | 0.279 ms | 0.545 ms | 1.317 ms |
| Xo 0.3 (wasip3) | 322,838 | 31.3 ms | 114,828 | 0.217 ms | 0.480 ms | 1.345 ms |
| Rust 0.3 (wasip3) | 91,295 | 31.8 ms | 121,758 | 0.201 ms | 0.458 ms | 1.340 ms |

Xo at 0.3 serves 1.21x the requests a second of Xo at 0.2 (every round:
1.21, 1.26, 1.15) and 0.943x Rust's at 0.3; p99 is the same within
noise for all three. First request times are 22 or 31 ms in every row
(the 5 ms poll and wasmtime's start), not a difference between them.
Stage 2's 0.2 row (257,324 bytes, 14.2 ms, 81,681 req/s) was the first
host at an older xo; this run reruns 0.2 on the same host as 0.3.

Raw output:

```
-- round 1
== xo02: 322996 bytes, validate ok
xo02 first request us: 72726 31286 32182 33022 24891 (median 32182); body: hello
c 32 requests 1902290 errors 0 rps 95114 p50_ms 0.279 p90_ms 0.547 p99_ms 1.317
-- round 1
== xo03: 322838 bytes, validate ok
xo03 first request us: 24409 24764 33106 34034 22186 (median 24764); body: hello
c 32 requests 2296561 errors 0 rps 114828 p50_ms 0.217 p90_ms 0.480 p99_ms 1.345
-- round 1
== rust03: 91295 bytes, validate ok
rust03 first request us: 31792 31794 34596 34162 31177 (median 31794); body: hello
c 32 requests 2435150 errors 0 rps 121758 p50_ms 0.201 p90_ms 0.458 p99_ms 1.340
-- round 2
== xo02: 322996 bytes, validate ok
xo02 first request us: 31120 30697 30117 30742 31067 (median 30742); body: hello
c 32 requests 1895088 errors 0 rps 94754 p50_ms 0.281 p90_ms 0.545 p99_ms 1.350
-- round 2
== xo03: 322838 bytes, validate ok
xo03 first request us: 31330 40417 30915 32179 30633 (median 31330); body: hello
c 32 requests 2389598 errors 0 rps 119480 p50_ms 0.209 p90_ms 0.463 p99_ms 1.284
-- round 2
== rust03: 91295 bytes, validate ok
rust03 first request us: 31340 22097 30822 30117 30732 (median 30732); body: hello
c 32 requests 2457417 errors 0 rps 122871 p50_ms 0.200 p90_ms 0.456 p99_ms 1.303
-- round 3
== xo02: 322996 bytes, validate ok
xo02 first request us: 32617 31307 22246 30084 40371 (median 31307); body: hello
c 32 requests 1964406 errors 0 rps 98220 p50_ms 0.272 p90_ms 0.523 p99_ms 1.293
-- round 3
== xo03: 322838 bytes, validate ok
xo03 first request us: 23269 22599 31314 31825 31588 (median 31314); body: hello
c 32 requests 2259671 errors 0 rps 112984 p50_ms 0.222 p90_ms 0.489 p99_ms 1.353
-- round 3
== rust03: 91295 bytes, validate ok
rust03 first request us: 34019 37535 34719 32960 34012 (median 34019); body: hello
c 32 requests 2424613 errors 0 rps 121231 p50_ms 0.202 p90_ms 0.460 p99_ms 1.360
```
