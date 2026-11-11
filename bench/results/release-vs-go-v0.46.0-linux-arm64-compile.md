# Xo against Go: xo 0.46.0 linux/arm64, compile and size (2026-11-11)

The released 0.46.0 (its linux-arm64 archive from the GitHub release) on linux/arm64 offload host 1,
SECTIONS=compile of scripts/release-vs-go.sh at main 0461a4f5d. It fills
the size column that release-vs-go-6fe10a957-linux-arm64.md left empty (its script read the size with BSD
stat on Linux). The script waited for the host's load to fall under 6 before it started (loads 6 to 9 then).

scripts/release-vs-go.sh on Linux host, 12 cores, 39 GB, Linux aarch64,
Go go1.26.1. Build times are wall milliseconds of one build; peak MB is the
build's largest resident set (cold / warm / edit). Warm is the third unchanged build
(xo's parts pack is written on the second and read from the third); second is the second.
Reports through v0.35.0 ran xo's timed builds under XO_CACHE_STATS=1, which added about
28 MB to coll's warm peak (116 against 88 MB, 2026-11-01), so xo's peak MB steps down
from those reports to this one with no change in the compiler.
The edit's path is what xo's cache statistics say an untimed build of another edit did: separate (against
std's variants, built after the cold row as a user's second build would have them),
with the std records, or whole (the whole program).
xo's builds run with the whole-build run cache off (XO_BIN_CACHE=off), so the second and
warm rows are compiles through the back-end caches, not a cached result (before 2026-10-29
they could be the run cache's 10 to 20 ms).
Size is the default build's binary.

## Compile and size

| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |
|---|---|---|---|---|---|---|---|---|
| llvm | go | 1271 | 19 | 19 | 72 | 293 / 23 / 70 | 2.29 | - |
| coll | go | 1277 | 21 | 19 | 77 | 303 / 23 / 69 | 2.30 | - |
| conc | go | 1291 | 20 | 20 | 67 | 291 / 23 / 70 | 2.30 | - |
| llvm-fib | xo | 1137 | 47 | 42 | 41 | 232 / 57 / 55 | 2.14 | separate, std records |
| llvm-sieve | xo | 1160 | 50 | 41 | 42 | 232 / 55 / 56 | 2.14 | separate, std records |
| llvm-nbody | xo | 1154 | 55 | 46 | 49 | 236 / 58 / 59 | 2.14 | separate, std records |
| llvm-sort | xo | 1163 | 49 | 43 | 44 | 229 / 58 / 56 | 2.14 | separate, std records |
| llvm-interp | xo | 1149 | 54 | 45 | 47 | 230 / 58 / 59 | 2.15 | separate, std records |
| llvm-collatz | xo | 1157 | 49 | 43 | 41 | 222 / 57 / 56 | 2.14 | separate, std records |
| llvm-strings | xo | 1146 | 51 | 40 | 41 | 226 / 55 / 55 | 2.14 | separate, std records |
| coll | xo | 1213 | 116 | 107 | 112 | 225 / 66 / 64 | 2.26 | separate, std records |
| conc-spawn | xo | 1338 | 49 | 45 | 45 | 225 / 58 / 56 | 2.24 | separate, std records |
| conc-fanout | xo | 1341 | 52 | 44 | 45 | 232 / 56 / 57 | 2.28 | separate, std records |
| conc-pingpong | xo | 1348 | 53 | 45 | 46 | 238 / 57 / 60 | 2.28 | separate, std records |
| conc-mutex | xo | 1343 | 53 | 47 | 47 | 226 / 59 / 59 | 2.32 | separate, std records |
| conc-rwmutex | xo | 1362 | 54 | 47 | 49 | 227 / 57 / 60 | 2.33 | separate, std records |
| conc-pmap | xo | 1352 | 53 | 47 | 47 | 231 / 60 / 57 | 2.28 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.
