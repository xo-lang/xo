# Build, size and startup of the language twins: 2026-11-22

Written by bench/perf/twins/build.sh. The CRUD server is bench/perf/http's users API in each language (Xo std/http, Go net/http, Rust axum on tokio, Node node:http, Python asyncio.Protocol and Starlette on uvicorn); hello prints one line.

## Machine and versions

- host 1, the linux/arm64 timing host (exclusive lease)
- Linux 7.0.14-linuxkit aarch64, 12 CPUs, 39 GB, load average at start 3.30
- xo 0.55.0 commit 9599fc7d key ff3b0011426efd26, NOT THIS CHECKOUT (its key is 019fa5963b5377a4)
- go version go1.26.11 linux/arm64
- cargo 1.99.1 (5f94df478 2026-08-27), rustc 1.99.1 (58cf8f9c3 2026-10-13)
- node v20.19.2 (/usr/bin/node, for tsc, esbuild and the startup row), typescript 5.9.3, esbuild 0.28.2, pinned in package-lock.json
- Python 3.13.5 (the virtual environment of bench/perf/python)

## Build and size

Wall milliseconds of one build; peak MB is the build's largest resident set (cold / warm / edit); size MB is the output as built (a binary, with DWARF for Xo and Go, none for Rust; the shipping-flag sizes are in What ships; Node's tsc output directory or esbuild bundle; Python has none).

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|---|
| crud | xo | 2810 | 10 | 10 | 630 | 764 / 28 / 542 | 9.27 |
| hello | xo | 1270 | 10 | 10 | 270 | 396 / 28 / 233 | 2.32 |
| crud | go | 3730 | 50 | 70 | 270 | 314 / 35 / 148 | 7.95 |
| hello | go | 2360 | 70 | 50 | 180 | 304 / 24 / 68 | 2.22 |
| crud | rust | 9580 | 30 | 20 | 2960 | 520 / 26 / 522 | 1.32 |
| hello | rust | 980 | 0 | 0 | 950 | 302 / 16 / 297 | 0.44 |
| project | node | 530 | 510 | 510 | 550 | 202 / 202 / 202 | 52.0 KB (dist) |
| bundle | node | 50 | 40 | 50 | 40 | 48 / 48 / 48 | 3.0 KB |
| server_asyncio | python | 30 | 30 | 20 | 30 | 18 / 17 / 17 | n/a |
| server_starlette | python | 50 | 50 | 50 | 50 | 28 / 27 / 27 | n/a |

Node single executable application (the bundle injected into a copy of the official node v20.19.2 binary with postject): 1510 ms, peak 715 MB, 93.50 MB.

## What ships

| side | what | MB |
|---|---|---|
| xo | the CRUD server binary, `xo build` (with DWARF) | 9.27 |
| xo | the CRUD server binary, `xo build --release` | 3.02 |
| go | the CRUD server binary, `go build -trimpath` (with DWARF) | 7.95 |
| go | the CRUD server binary, `go build -trimpath -ldflags="-s -w"` | 5.50 |
| rust | the CRUD server binary, `cargo build --release` (no debug info) | 1.32 |
| xo | the hello binary, `xo build` (with DWARF) | 2.32 |
| xo | the hello binary, `xo build --release` | 0.27 |
| go | the hello binary, `go build -trimpath` (with DWARF) | 2.22 |
| go | the hello binary, `go build -trimpath -ldflags="-s -w"` | 1.50 |
| rust | the hello binary, `cargo build --release` (no debug info) | 0.44 |
| node | the esbuild bundle of the server (needs a node runtime) | 3.0 KB |
| node | the official node v20.19.2 binary (the runtime) | 93.38 |
| node | the single executable application | 93.50 |
| python | the server sources (http/*.py) | 7.9 KB |
| python | the virtual environment's site-packages (uvicorn, Starlette, uvloop, httptools and their dependencies) | 30.21 |
| python | the interpreter binary and its standard library (/usr/bin/python3.13, /usr/lib/python3.13) | 35.52 |

## Startup (hello, `xo bench --ab`, 20 rounds, A = the other language, B = Xo)

| hello, go | load 3.50 |
    wall_ms               0.61        0.522       7.57% 9.803%      0.855   0.816 to 0.909    0.851 0.862     too short (n=20)
    cpu_ms                0.629       0.472       6.757% 8.59%      0.75    0.71 to 0.786     0.742 0.754     too coarse (n=20)
    max_rss_kb            4644        968.0       44.27% 0%         0.208   0.17 to 0.267     0.266 0.17      better (n=20)
| hello, rust | load 3.50 |
    wall_ms               0.308       0.532       9.71% 8.487%      1.727   1.567 to 1.817    1.69 1.732      too short (n=20)
    cpu_ms                0.251       0.49        12.35% 7.704%     1.952   1.761 to 2.08     1.88 1.966      too coarse (n=20)
    max_rss_kb            1504        1320        0.332% 0.379%     0.878   0.875 to 0.879    0.878 0.876     better (n=20)
| hello, node | load 3.50 |
    wall_ms               45.27       0.652       2.605% 12.96%     0.014   0.013 to 0.015    0.013 0.015     too short (n=20)
    cpu_ms                45.98       0.571       2.573% 10.08%     0.012   0.012 to 0.013    0.012 0.013     too coarse (n=20)
    max_rss_kb            49888       968.0       0.118% 0%         0.019   0.019 to 0.019    0.019 0.019     better (n=20)
| hello, python | load 3.30 |
    wall_ms               5.607       0.563       3.06% 6.09%       0.1     0.097 to 0.103    0.1 0.101       too short (n=20)
    cpu_ms                5.531       0.517       2.839% 5.179%     0.093   0.091 to 0.096    0.092 0.094     too coarse (n=20)
    max_rss_kb            7664        968.0       0.065% 0%         0.126   0.126 to 0.126    0.126 0.126     better (n=20)
