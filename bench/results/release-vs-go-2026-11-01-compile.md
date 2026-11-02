# Xo against Go: xo 0.35.0 darwin/arm64 (2026-11-01)

scripts/release-vs-go.sh on Darwin arm64, 24 GB,
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
| llvm | go | 1560 | 40 | 30 | 100 | 296 / 25 / 69 | 2.39 | - |
| coll | go | 1490 | 40 | 40 | 110 | 294 / 26 / 68 | 2.43 | - |
| conc | go | 1560 | 40 | 40 | 100 | 294 / 26 / 67 | 2.40 | - |
| llvm-fib | xo | 1010 | 60 | 40 | 40 | 334 / 74 / 76 | 1.27 | separate, std records |
| llvm-sieve | xo | 910 | 50 | 40 | 40 | 335 / 74 / 76 | 1.27 | separate, std records |
| llvm-nbody | xo | 930 | 60 | 50 | 50 | 334 / 77 / 77 | 1.27 | separate, std records |
| llvm-sort | xo | 910 | 50 | 40 | 40 | 336 / 76 / 77 | 1.27 | separate, std records |
| llvm-interp | xo | 920 | 60 | 40 | 50 | 336 / 77 / 79 | 1.27 | separate, std records |
| llvm-collatz | xo | 920 | 50 | 40 | 40 | 335 / 75 / 76 | 1.27 | separate, std records |
| llvm-strings | xo | 930 | 50 | 40 | 40 | 336 / 76 / 76 | 1.27 | separate, std records |
| coll | xo | 960 | 130 | 100 | 110 | 340 / 85 / 84 | 1.38 | separate, std records |
| conc-spawn | xo | 1080 | 60 | 40 | 40 | 333 / 75 / 77 | 1.36 | separate, std records |
| conc-fanout | xo | 1150 | 50 | 40 | 40 | 333 / 75 / 77 | 1.36 | separate, std records |
| conc-pingpong | xo | 1090 | 60 | 40 | 50 | 333 / 79 / 79 | 1.38 | separate, std records |
| conc-mutex | xo | 1060 | 60 | 40 | 40 | 333 / 77 / 79 | 1.42 | separate, std records |
| conc-rwmutex | xo | 1070 | 60 | 40 | 40 | 333 / 77 / 78 | 1.42 | separate, std records |
| conc-pmap | xo | 1120 | 60 | 40 | 40 | 333 / 77 / 79 | 1.36 | separate, std records |

The Go column builds one binary per group (llvm, coll, conc); the Xo
programs are one per workload except coll.

## Run (xo bench --ab, A = Go, B = Xo, 3 rounds)

