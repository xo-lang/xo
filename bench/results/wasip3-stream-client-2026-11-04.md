# Decision 0447 stage 5 step 2: streamed bodies and the client at WASI 0.3 against 0.2 and Rust (2026-11-04)

Second linux/arm64 offload host (18 cpus), alone on it (an exclusive
lease), wasmtime 50.0.1 for every row. Each component serves:

- `POST /echo`: reads the request body as it arrives and answers
  `len N`. The load sends 100,000-byte bodies (want `len 100000`).
- `GET /proxy`: sends one GET to a Go server on 127.0.0.1:18999 and
  answers with its body (want `upstream`).

How each side was built:

- Xo 0.2 and Xo 0.3: the same program (`handle_stream` for the echo,
  `http.client(...).send` for the proxy), built by xo at commit 688119751
  of branch l4-s5b with `--targets=wasip2/wasm` and
  `--targets=wasip3/wasm`.
- Rust 0.3: the `wasip3` crate 0.10.0 with wit-bindgen 0.63.0
  (async-spawn); `Request::consume_body` read to the end, and
  `wasip3::http::client::send` then `Response::consume_body`. Built with
  `cargo build --release --target wasm32-wasip2`, `opt-level = "s"`, LTO,
  stripped.
- Load: loadgen3 -c 32 -d 20s per route; it fails on any other body.
  Three rounds, the three components in turn each round.

Medians of the three rounds:

| component | bytes | echo req/s | echo p50 | echo p99 | proxy req/s | proxy p50 | proxy p99 |
|---|---|---|---|---|---|---|---|
| Xo 0.2 (wasip2) | 531,394 | 21,943 | 1.393 ms | 3.119 ms | 5,877 | 5.348 ms | 8.988 ms |
| Xo 0.3 (wasip3) | 530,701 | 57,056 | 0.333 ms | 2.527 ms | 17,587 | 0.872 ms | 10.175 ms |
| Rust 0.3 (wasip3) | 119,123 | 56,102 | 0.336 ms | 2.574 ms | 17,788 | 0.871 ms | 10.711 ms |

Results:

- Echo: Xo at 0.3 serves 1.017x Rust's requests a second, and 2.60x Xo
  at 0.2.
- Proxy: Xo at 0.3 serves 0.989x Rust's, and 2.99x Xo at 0.2.
- Latency at 0.3 matches Rust's at p50 and p99 on both routes.
- Xo at 0.2 has the lower proxy p99 but a third of the rate.
- Size: the client and handle_stream take Xo's component from 323 KB
  (hello) to 531 KB; Rust's is 4.5x smaller.

Raw output:

```
host: 11c7eb217fae 18 cpus; wasmtime 50.0.1 (c307ec81d 2026-10-29); xo 688119751
== round 1 xo02: 531394 bytes, validate ok
xo02 echo: c 32 requests 409978 errors 0 rps 20499 p50_ms 1.499 p90_ms 2.367 p99_ms 3.253
xo02 proxy: c 32 requests 114033 errors 0 rps 5702 p50_ms 5.502 p90_ms 7.332 p99_ms 9.292
== round 1 xo03: 530701 bytes, validate ok
xo03 echo: c 32 requests 1141110 errors 0 rps 57056 p50_ms 0.333 p90_ms 1.301 p99_ms 2.527
xo03 proxy: c 32 requests 351740 errors 0 rps 17587 p50_ms 0.871 p90_ms 5.149 p99_ms 9.816
== round 1 rust03: 119123 bytes, validate ok
rust03 echo: c 32 requests 1123220 errors 0 rps 56161 p50_ms 0.339 p90_ms 1.328 p99_ms 2.532
rust03 proxy: c 32 requests 353459 errors 0 rps 17673 p50_ms 0.882 p90_ms 5.146 p99_ms 10.711
== round 2 xo02: 531394 bytes, validate ok
xo02 echo: c 32 requests 453574 errors 0 rps 22679 p50_ms 1.354 p90_ms 2.140 p99_ms 2.963
xo02 proxy: c 32 requests 124086 errors 0 rps 6204 p50_ms 5.067 p90_ms 6.744 p99_ms 8.494
== round 2 xo03: 530701 bytes, validate ok
xo03 echo: c 32 requests 1144075 errors 0 rps 57204 p50_ms 0.329 p90_ms 1.309 p99_ms 2.484
xo03 proxy: c 32 requests 358702 errors 0 rps 17935 p50_ms 0.872 p90_ms 5.062 p99_ms 10.175
== round 2 rust03: 119123 bytes, validate ok
rust03 echo: c 32 requests 1118029 errors 0 rps 55901 p50_ms 0.336 p90_ms 1.339 p99_ms 2.574
rust03 proxy: c 32 requests 355769 errors 0 rps 17788 p50_ms 0.871 p90_ms 5.062 p99_ms 11.224
== round 3 xo02: 531394 bytes, validate ok
xo02 echo: c 32 requests 438855 errors 0 rps 21943 p50_ms 1.393 p90_ms 2.214 p99_ms 3.119
xo02 proxy: c 32 requests 117535 errors 0 rps 5877 p50_ms 5.348 p90_ms 7.143 p99_ms 8.988
== round 3 xo03: 530701 bytes, validate ok
xo03 echo: c 32 requests 1045136 errors 0 rps 52257 p50_ms 0.359 p90_ms 1.437 p99_ms 2.713
xo03 proxy: c 32 requests 343444 errors 0 rps 17172 p50_ms 0.889 p90_ms 5.167 p99_ms 11.677
== round 3 rust03: 119123 bytes, validate ok
rust03 echo: c 32 requests 1122048 errors 0 rps 56102 p50_ms 0.336 p90_ms 1.319 p99_ms 2.604
rust03 proxy: c 32 requests 357232 errors 0 rps 17862 p50_ms 0.863 p90_ms 5.237 p99_ms 9.512
```
