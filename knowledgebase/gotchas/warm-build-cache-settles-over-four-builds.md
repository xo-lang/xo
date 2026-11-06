# A warm build's cache writes settle only after four builds

Status: Observed (2026-11-06, linux/arm64 host 2, xo-lang-4)

## Symptom

A measurement of what a warm, unchanged build writes to xo's caches
(XO_CACHE_STATS=keys) gave different answers run to run: 3.6 MB once,
111 KB once (the borrow record and "seen" markers), 167 KB another time
(the front end's neg list in the target's and the shared pack, plus three
fe entries). TestWarmBuildWritesLittle failed after three builds.

## Cause

A fresh cache settles in stages. In a private cache, a hello's builds
wrote (f1ace59cf, XO_SEPCOMP=0 and the default alike):

| build | written |
|---|---|
| 1 | 35.7 MB (entries, the shared pack) |
| 2 | 19.3 MB (the target's packs form) |
| 3 | 2.4 MB (the last pack segment) |
| 4 | 262 KB (fe/neg, decision 0351's list of keys found nowhere, in the target's and the shared pack) |
| 5 on | 1,352 bytes (the build information object) |

A shared cache that other builds touch can be at any of these stages.

## Fix

Measure a warm build's writes after four builds of the program in a
private cache (XO_BACKEND_CACHE_DIR), as TestWarmBuildWritesLittle does,
and list the writes (XO_CACHE_STATS=keys) before reading a total.
