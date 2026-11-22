# Rust against Xo: 2026-11-22 (HTTP rows)

Written by bench/perf/rust/run.sh (twins/lib.sh). Ratios are Xo over Rust (below 1: Xo is faster or smaller).

## Machine and versions

- Linux 7.0.14-linuxkit aarch64, host 1 (the linux/arm64 timing host, exclusive lease), 12 CPUs, load average at start 0.08
- xo 0.55.0 commit 9599fc7d key ff3b0011426efd26, NOT THIS CHECKOUT (its key is )
- go version go1.26.11 linux/arm64
- cargo 1.99.1 (5f94df478 2026-08-27), rustc 1.99.1 (58cf8f9c3 2026-10-13)
- crates (Cargo.lock): axum 0.8.9;hyper 1.12.0;serde 1.0.229;serde_json 1.0.154;tokio 1.54.1;
- release profile of every crate: opt-level 3, codegen-units 1, thin LTO, panic abort


## HTTP: the users CRUD API (axum 0.8 on tokio with serde_json; Go net/http; Xo std/http)

```
# config rps p50_us p99_us p999_us errors gcs pause_ms peak_heap_mb gc_cpu_pct peak_rss_mb gcs_per_10k cpu_us_req
go 121682 188 1204 2916 0 3051 698.40 3 4 19.5 20.92 27.92
xo-arenas 178741 140 904 2487 0 0 0.00 0 0 13.9 0.00 12.13
rust 202689 122 888 2112 0 0 0.00 0 0 6.0 0.00 7.90
go 122458 188 1198 2984 0 2849 659.80 3 4 19.4 19.35 27.51
xo-arenas 179519 142 924 2451 0 0 0.00 0 0 14.2 0.00 11.78
rust 202380 122 893 2096 0 0 0.00 0 0 5.9 0.00 7.80
go 123038 187 1199 2638 0 2952 677.22 3 4 19.3 20.01 27.20
xo-arenas 179838 141 913 2262 0 0 0.00 0 0 14.5 0.00 11.88
rust 204166 122 886 1869 0 0 0.00 0 0 6.1 0.00 7.81

Medians of 3 runs, 32 workers, 10s measured after 2s warmup, loopback.

| Config | Req/s | p50 us | p99 us | p99.9 us | Errors | GCs | GC pause ms | Peak heap MB | GC CPU % | Peak RSS MB | GCs/10k req | CPU us/req |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| go | 122458 | 188 | 1199 | 2916 | 0 | 2952 | 677.22 | 3 | 4 | 19.4 | 20.01 | 27.51 |
| xo-arenas | 179519 | 141 | 913 | 2451 | 0 | 0 | 0.00 | 0 | 0 | 14.2 | 0.00 | 11.88 |
| rust | 202689 | 122 | 888 | 2096 | 0 | 0 | 0.00 | 0 | 0 | 6.0 | 0.00 | 7.81 |
```
