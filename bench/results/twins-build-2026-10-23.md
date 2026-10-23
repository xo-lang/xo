# Build, size and startup of the language twins: 2026-10-23

Written by bench/perf/twins/build.sh. The CRUD server is bench/perf/http's users API in each language (Xo std/http, Go net/http, Rust axum on tokio, Node node:http, Python asyncio.Protocol and Starlette on uvicorn); hello prints one line.

## Machine and versions

- Darwin 27.0.0 arm64, Apple M4 Pro, 12 CPUs, 24 GB, load average at start 4.15
- xo 0.27.0 commit b6cfa83c key faf0f5710646b132, NOT THIS CHECKOUT (its key is 4d318b1a392ed239)
- go version go1.26.1 darwin/arm64
- cargo 1.99.1 (5f94df478 2026-08-27), rustc 1.99.1 (58cf8f9c3 2026-10-13)
- node v22.22.1 (Homebrew, for tsc, esbuild and the startup row), typescript 5.9.3, esbuild 0.28.2, pinned in package-lock.json
- Python 3.13.12 (the virtual environment of bench/perf/python)

## Build and size

Wall milliseconds of one build; peak MB is the build's largest resident set (cold / warm / edit); size MB is the output (a binary, Node's tsc output directory or esbuild bundle; Python has none).

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB |
|---|---|---|---|---|---|---|---|
| crud | xo | 3940 | 710 | 570 | 570 | 960 / 541 / 538 | 6.13 |
| hello | xo | 1880 | 370 | 280 | 280 | 491 / 301 / 307 | 1.85 |
| crud | go | 3070 | 80 | 80 | 240 | 310 / 33 / 152 | 8.03 |
| hello | go | 1480 | 40 | 40 | 90 | 315 / 25 / 68 | 2.36 |
| crud | rust | 8119 | 30 | 30 | 3020 | 408 / 41 / 399 | 1.12 |
| hello | rust | 1310 | 10 | 10 | 1230 | 190 / 20 / 190 | 0.41 |
| project | node | 1090 | 540 | 530 | 540 | 199 / 201 / 201 | 28.0 KB (dist) |
| bundle | node | 390 | 170 | 180 | 170 | 76 / 76 / 76 | 3.0 KB |
| server_asyncio | python | 200 | 40 | 40 | 40 | 23 / 22 / 22 | n/a |
| server_starlette | python | 190 | 70 | 70 | 70 | 32 / 31 / 31 | n/a |

Node single executable application (the bundle injected into a copy of the official node v22.22.1 binary with postject, signed ad hoc): 4230 ms, peak 886 MB, 106.87 MB.

## What ships

| side | what | MB |
|---|---|---|
| xo | the CRUD server binary | 6.13 |
| go | the CRUD server binary | 8.03 |
| rust | the CRUD server binary | 1.12 |
| node | the esbuild bundle of the server (needs a node runtime) | 3.0 KB |
| node | the official node v22.22.1 binary (the runtime) | 107.74 |
| node | the single executable application | 106.87 |
| python | the server sources (http/*.py) | 7.9 KB |
| python | the virtual environment's site-packages (uvicorn, Starlette, uvloop, httptools and their dependencies) | 25.77 |
| python | the interpreter's installation (/opt/homebrew/opt/python@3.13/Frameworks/Python.framework/Versions/3.13) | 77.71 |

## Startup (hello, `xo bench --ab`, 20 rounds, A = the other language, B = Xo)

| hello, go | load 4.42 |
    wall_ms               3.214       2.913       5.298% 7.981%     0.906   0.879 to 0.95     better (n=20)
    cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=20)
    max_rss_kb            4240        2608        2.264% 1.227%     0.615   0.611 to 0.625    better (n=20)
| hello, rust | load 4.42 |
    wall_ms               2.788       2.911       5.577% 5.283%     1.044   1.004 to 1.073    worse (n=20)
    cpu_ms                0           0           0% 0%             1.0     1.0 to 1.0        too coarse (n=20)
    max_rss_kb            1840        2576        0% 1.242%         1.4     1.4 to 1.417      worse (n=20)
| hello, node | load 4.42 |
    wall_ms               49.9        3.241       2.448% 10.23%     0.065   0.062 to 0.067    better (n=20)
    cpu_ms                40.0        0           0% 0%             0       0 to 0            too coarse (n=20)
    max_rss_kb            36568       2600        0.241% 1.846%     0.071   0.07 to 0.072     better (n=20)
| hello, python | load 4.15 |
    wall_ms               19.57       3.039       3.366% 4.656%     0.155   0.153 to 0.159    better (n=20)
    cpu_ms                10.0        0           0% 0%             0       0 to 0            too coarse (n=20)
    max_rss_kb            13936       2624        0.115% 1.829%     0.188   0.185 to 0.189    better (n=20)

## Notes

(Added by hand after the run.)

- The xo is the released v0.27.0 darwin/arm64 archive, unpacked (public commit b6cfa83c), run with XO_BENCH_ANY=1: "NOT THIS CHECKOUT" above means it is not a build of this commit's sources, which is intended. Its rows match release-vs-go-v0.27.0.md's hello-sized programs (1.8 to 2.3 s cold, 0.27 to 0.34 s warm).
- Not a quiet machine: other agents' jobs ran (load 4 to 5). Every build group waited for a load under 5. A first run of this script earlier the same day (before a fix to how small sizes print) differed by more than 10% on: Rust's cold hello (2040 ms against 1310) and crud edit (3940 against 3020), Xo's crud warm and edit builds (510 and 500 against 570 and 570), Node's cold tsc (970 against 1090), Starlette's first import (310 against 190) and the single executable build (5140 against 4230); rows under 100 ms move in steps of 10 ms, /usr/bin/time's resolution.
- Rust builds are `--release` with the twins' profile (opt-level 3, one codegen unit, thin LTO), as the measured binaries were; a release build does not compile incrementally, so an edit rebuilds the crate (3.0 s for the axum server). The cold crud build compiles axum, hyper, tokio and serde from the downloaded sources.
- Node: tsc compiles all of bench/perf/node/src (the server, JSON, concurrency, fib and hello twins), with no cache between builds. The single executable application is about the size of the Node binary it copies; it was checked by starting it and creating a user (HTTP 201).
- Python has no build step; the rows time `import server_asyncio` and `import server_starlette`, the first with no __pycache__ (CPython compiles the module then; the installed packages were compiled at install).
- Startup is the wall time of the whole hello process, start to exit, so it includes Node's and CPython's start.
