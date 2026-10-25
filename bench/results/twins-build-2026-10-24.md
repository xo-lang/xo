# Build, size and startup of the language twins: 2026-10-24

Written by bench/perf/twins/build.sh. The CRUD server is bench/perf/http's users API in each language (Xo std/http, Go net/http, Rust axum on tokio, Node node:http, Python asyncio.Protocol and Starlette on uvicorn); hello prints one line.

## Machine and versions

- Darwin 27.0.0 arm64, Apple M4 Pro, 12 CPUs, 24 GB, load average at start 4.22
- xo 0.29.0 commit e7de76b1 key 112b09aa7ba1e90f, NOT THIS CHECKOUT (its key is 7a95a103705157dc)
- go version go1.26.1 darwin/arm64
- cargo 1.99.1 (5f94df478 2026-08-27), rustc 1.99.1 (58cf8f9c3 2026-10-13)
- node v22.22.1 (Homebrew, for tsc, esbuild and the startup row), typescript 5.9.3, esbuild 0.28.2, pinned in package-lock.json
- Python 3.13.12 (the virtual environment of bench/perf/python)

## Build and size

Wall milliseconds of one build; peak MB is the build's largest resident set (cold / warm / edit); size MB is the output (a binary, Node's tsc output directory or esbuild bundle; Python has none).

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|---|
| crud | xo | 2440 | 30 | 30 | 520 | 984 / 14 / 547 | 6.23 |
| hello | xo | 1150 | 30 | 30 | 290 | 506 / 14 / 313 | 1.90 |
| crud | go | 2940 | 80 | 80 | 240 | 303 / 32 / 151 | 8.03 |
| hello | go | 1420 | 30 | 40 | 100 | 315 / 25 / 69 | 2.36 |
| crud | rust | 7910 | 30 | 30 | 2940 | 409 / 41 / 413 | 1.12 |
| hello | rust | 1270 | 10 | 10 | 1220 | 191 / 20 / 193 | 0.41 |
| project | node | 920 | 560 | 540 | 530 | 207 / 200 / 200 | 28.0 KB (dist) |
| bundle | node | 360 | 170 | 170 | 170 | 75 / 76 / 76 | 3.0 KB |
| server_asyncio | python | 70 | 40 | 40 | 40 | 23 / 22 / 22 | n/a |
| server_starlette | python | 230 | 70 | 70 | 70 | 32 / 31 / 31 | n/a |

Node single executable application (the bundle injected into a copy of the official node v22.22.1 binary with postject, signed ad hoc): 5060 ms, peak 884 MB, 106.87 MB.

## What ships

| side | what | MB |
|---|---|---|
| xo | the CRUD server binary | 6.23 |
| go | the CRUD server binary | 8.03 |
| rust | the CRUD server binary | 1.12 |
| node | the esbuild bundle of the server (needs a node runtime) | 3.0 KB |
| node | the official node v22.22.1 binary (the runtime) | 107.74 |
| node | the single executable application | 106.87 |
| python | the server sources (http/*.py) | 7.9 KB |
| python | the virtual environment's site-packages (uvicorn, Starlette, uvloop, httptools and their dependencies) | 25.76 |
| python | the interpreter's installation (/opt/homebrew/opt/python@3.13/Frameworks/Python.framework/Versions/3.13) | 77.71 |

## Startup (hello, `xo bench --ab`, 20 rounds, A = the other language, B = Xo)

| hello, go | load 2.49 |
    wall_ms               3.498       3.045       9.664% 8.047%     0.87    0.844 to 0.929    better (n=20)
    cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=20)
    max_rss_kb            4192        2592        2.672% 0%         0.618   0.616 to 0.628    better (n=20)
| hello, rust | load 2.49 |
    wall_ms               2.764       2.885       7.417% 7.106%     1.044   1.012 to 1.075    worse (n=20)
    cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=20)
    max_rss_kb            1824        2592        0% 0%             1.421   1.421 to 1.421    worse (n=20)
| hello, node | load 2.49 |
    wall_ms               47.39       3.008       1.617% 10.52%     0.063   0.06 to 0.066     better (n=20)
    cpu_ms                30.0        0           0% 0%             0       0 to 0            too coarse (n=20)
    max_rss_kb            36536       2600        0.263% 0.615%     0.071   0.071 to 0.071    better (n=20)
| hello, python | load 4.22 |
    wall_ms               18.81       2.873       1.721% 6.275%     0.153   0.15 to 0.156     better (n=20)
    cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=20)
    max_rss_kb            13936       2608        0.258% 1.84%      0.187   0.186 to 0.189    better (n=20)
