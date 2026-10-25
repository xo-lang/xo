# Build, size and startup of the language twins: 2026-10-24

Written by bench/perf/twins/build.sh. The CRUD server is bench/perf/http's users API in each language (Xo std/http, Go net/http, Rust axum on tokio, Node node:http, Python asyncio.Protocol and Starlette on uvicorn); hello prints one line.

## Machine and versions

- Darwin 27.0.0 arm64, Apple M4 Pro, 12 CPUs, 24 GB, load average at start 3.74
- xo 0.29.0 commit e7de76b1 key 112b09aa7ba1e90f, NOT THIS CHECKOUT (its key is bd64c24ce08debf2)
- go version go1.26.1 darwin/arm64
- cargo 1.99.1 (5f94df478 2026-08-27), rustc 1.99.1 (58cf8f9c3 2026-10-13)
- node v22.22.1 (Homebrew, for tsc, esbuild and the startup row), typescript 5.9.3, esbuild 0.28.2, pinned in package-lock.json
- Python 3.13.12 (the virtual environment of bench/perf/python)

## Build and size

Wall milliseconds of one build; peak MB is the build's largest resident set (cold / warm / edit); size MB is the output as built (a binary, with DWARF for Xo and Go, none for Rust; the shipping-flag sizes are in What ships; Node's tsc output directory or esbuild bundle; Python has none).

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|---|
| crud | xo | 2280 | 30 | 30 | 500 | 992 / 14 / 547 | 6.23 |
| hello | xo | 1130 | 30 | 30 | 280 | 504 / 14 / 311 | 1.90 |
| crud | go | 2930 | 80 | 80 | 240 | 311 / 33 / 151 | 8.03 |
| hello | go | 1420 | 30 | 30 | 90 | 308 / 25 / 67 | 2.36 |
| crud | rust | 7550 | 30 | 30 | 2910 | 410 / 41 / 419 | 1.12 |
| hello | rust | 1250 | 10 | 10 | 1200 | 192 / 20 / 193 | 0.41 |
| project | node | 690 | 520 | 520 | 520 | 206 / 201 / 200 | 28.0 KB (dist) |
| bundle | node | 370 | 170 | 160 | 160 | 76 / 76 / 76 | 3.0 KB |
| server_asyncio | python | 40 | 40 | 40 | 40 | 23 / 22 / 22 | n/a |
| server_starlette | python | 70 | 70 | 70 | 70 | 32 / 31 / 31 | n/a |

Node single executable application (the bundle injected into a copy of the official node v22.22.1 binary with postject, signed ad hoc): 4890 ms, peak 885 MB, 106.87 MB.

## What ships

Sizes of one build with the flags named: `xo build` and `go build` keep DWARF (Xo's is about 4.1 MB of the CRUD binary), `--release` and `-s -w` drop it, and cargo's release profile has none. Measured by build.sh from the same sources as the rows above; the build times above are of the plain builds. Rerun 2026-10-24 21:39 for these rows only.

| side | what | MB |
|---|---|---|
| xo | the CRUD server binary, `xo build` (with DWARF) | 6.23 |
| xo | the CRUD server binary, `xo build --release` | 2.28 |
| go | the CRUD server binary, `go build -trimpath` (with DWARF) | 8.03 |
| go | the CRUD server binary, `go build -trimpath -ldflags="-s -w"` | 5.48 |
| rust | the CRUD server binary, `cargo build --release` (no debug info) | 1.12 |
| xo | the hello binary, `xo build` (with DWARF) | 1.90 |
| xo | the hello binary, `xo build --release` | 0.67 |
| go | the hello binary, `go build -trimpath` (with DWARF) | 2.36 |
| go | the hello binary, `go build -trimpath -ldflags="-s -w"` | 1.57 |
| rust | the hello binary, `cargo build --release` (no debug info) | 0.41 |
| node | the esbuild bundle of the server (needs a node runtime) | 3.0 KB |
| node | the official node v22.22.1 binary (the runtime) | 107.74 |
| node | the single executable application | 106.87 |
| python | the server sources (http/*.py) | 7.9 KB |
| python | the virtual environment's site-packages (uvicorn, Starlette, uvloop, httptools and their dependencies) | 25.76 |
| python | the interpreter's installation (/opt/homebrew/opt/python@3.13/Frameworks/Python.framework/Versions/3.13) | 77.71 |

## Startup (hello, `xo bench --ab`, 20 rounds, A = the other language, B = Xo)

| hello, go | load 3.98 |
    wall_ms               2.849       2.605       8.415% 5.509%     0.914   0.883 to 0.936    better (n=20)
    cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=20)
    max_rss_kb            4192        2592        2.672% 0%         0.618   0.616 to 0.628    better (n=20)
| hello, rust | load 3.98 |
    wall_ms               2.483       2.572       6.444% 10.26%     1.036   1.016 to 1.115    worse (n=20)
    cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=20)
    max_rss_kb            1824        2600        0% 0.615%         1.425   1.421 to 1.43     worse (n=20)
| hello, node | load 3.98 |
    wall_ms               46.16       2.815       1.879% 3.854%     0.061   0.06 to 0.062     better (n=20)
    cpu_ms                30.0        0           0% 0%             0       0 to 0            too coarse (n=20)
    max_rss_kb            36488       2592        0.362% 0.154%     0.071   0.071 to 0.071    better (n=20)
| hello, python | load 3.74 |
    wall_ms               17.53       2.678       2.201% 8.242%     0.153   0.145 to 0.158    better (n=20)
    cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=20)
    max_rss_kb            13936       2600        0.172% 1.846%     0.187   0.186 to 0.188    better (n=20)

## Three runs (quiet.sh wait 2 first; the table above is run 3)

Three consecutive runs of bench/perf/twins/build.sh with the 0.29.0 xo (e7de76b1), 2026-10-24 21:04 to 21:08. Load average (1 minute) at each run's start from uptime: run 1 1.98, run 2 4.28, run 3 3.41 (the machine's own builds raise it, so the file's "load average at start" lines read higher than the wait's 2). The website uses the best of the three per row. Milliseconds: cold / second / warm / edit.

| program | side | run 1 | run 2 | run 3 | best |
|---|---|---|---|---|---|
| crud | xo | 2260 / 30 / 30 / 500 | 2290 / 40 / 30 / 510 | 2280 / 30 / 30 / 500 | 2260 / 30 / 30 / 500 |
| hello | xo | 1110 / 30 / 30 / 280 | 1130 / 30 / 30 / 280 | 1130 / 30 / 30 / 280 | 1110 / 30 / 30 / 280 |
| crud | go | 2900 / 80 / 80 / 240 | 2910 / 80 / 80 / 240 | 2930 / 80 / 80 / 240 | 2900 / 80 / 80 / 240 |
| hello | go | 1400 / 30 / 30 / 90 | 1400 / 30 / 30 / 90 | 1420 / 30 / 30 / 90 | 1400 / 30 / 30 / 90 |
| crud | rust | 7570 / 30 / 30 / 2880 | 7650 / 30 / 30 / 2900 | 7550 / 30 / 30 / 2910 | 7550 / 30 / 30 / 2880 |
| hello | rust | 1260 / 10 / 10 / 1200 | 1270 / 10 / 10 / 1200 | 1250 / 10 / 10 / 1200 | 1250 / 10 / 10 / 1200 |

Go's edit build (240 ms for crud, 90 ms for hello) is faster than Xo's (500 and 280 ms) in every run. Xo's startup against Go's hello (wall ratio): 0.898, 0.903, 0.914.
