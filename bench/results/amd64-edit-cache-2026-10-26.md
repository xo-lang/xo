# amd64 edit rebuilds with the per-function and part cache (2026-10-26)

Milestone 0.33.0, criteria 2 and 3: the first gap of
amd64-parity-audit-2026-10-26.md. `--backend=amd64` builds now use the
per-function and part back-end cache, lazy inline-cache decode (decision
0283 stage B) and deferred inline verification, as arm64 builds have
(compiler/xo/main.xo build_native, defer_inline_verify; xo-lang-4).

Method: one M-series Mac, `--phase-times`, cross builds with
`--targets=T` from the Mac (an x86 ELF and an x86_64 Mach-O; the build
does the same work on any host). One private XO_BACKEND_CACHE_DIR per
program and target: a cold build, a second and a warm one, then an edit
and a body edit, each timed wall. Programs: bench/perf/llvm/xo/fib and
bench/perf/http/xo (the HTTP server). Edits: fib `if n < 2` to
`if n < 3` (edit), then back (body); the server gets an unused function
appended (edit), then the reachable `log.info("listening", ...)` in
main becomes `"listening on"` (body). After the body edit, the binary
is compared with a cold build of the same source in a fresh cache (same
file name, since an ad hoc signature names it): "same" in every row of
both tables. Before: xo at abfac9234; after: that commit plus this
change.

Before (wall ms; peak MB of cold/warm/edit/body):

| program | target | cold | second | warm | edit | body | peak MB | body = cold |
|---|---|---|---|---|---|---|---|---|
| fib | darwin/arm64 | 1056 | 35 | 30 | 278 | 264 | 484/10/286/274 | same |
| fib | linux/amd64 | 1065 | 34 | 30 | 485 | 465 | 488/10/318/317 | same |
| fib | darwin/amd64 | 1014 | 33 | 31 | 461 | 446 | 468/10/309/311 | same |
| http | darwin/arm64 | 2413 | 35 | 33 | 611 | 596 | 958/10/547/508 | same |
| http | linux/amd64 | 2742 | 35 | 32 | 1133 | 1244 | 930/10/544/546 | same |
| http | darwin/amd64 | 2620 | 36 | 32 | 1071 | 1187 | 898/10/526/520 | same |

After:

| program | target | cold | second | warm | edit | body | peak MB | body = cold |
|---|---|---|---|---|---|---|---|---|
| fib | darwin/arm64 | 1063 | 33 | 32 | 285 | 267 | 484/10/279/274 | same |
| fib | linux/amd64 | 1080 | 33 | 31 | 305 | 290 | 516/10/291/286 | same |
| fib | darwin/amd64 | 1029 | 34 | 30 | 289 | 269 | 495/10/286/273 | same |
| http | darwin/arm64 | 2417 | 37 | 32 | 617 | 595 | 957/10/540/514 | same |
| http | linux/amd64 | 2744 | 36 | 33 | 640 | 676 | 991/10/560/527 | same |
| http | darwin/amd64 | 2652 | 36 | 32 | 613 | 643 | 962/10/546/514 | same |

The HTTP server's body edit on linux/amd64: phase emit 555 ms to 28 ms,
`cache functions 1484/1485 reused`, `cache parts 38/39 reused`; phase
inline 148 ms to 65 ms (the deferred verification: inline.verify 61 to
18 ms). amd64 edit and body builds are now within 1 to 14% of arm64's
(they were 1.7x to 2.1x). The cold build's peak grows 30 to 64 MB on
amd64, the cache entries it now writes, as on arm64. One run of each
row; the arm64 rows, unchanged by the change, show the run to run
noise (up to 2%).

Proof of identity: internal/arm64 TestBackendCacheEditIdentity now
also builds linux/amd64, darwin/amd64 and windows/amd64 (five edits of a
300-function program, each cached build byte for byte the cache-off
build, and failing if no function or part was read from the cache).

Not windows/amd64: its per-function and part cache stays off (a cached
function's C thunks were missing, gotchas/fn-cache-hit-skips-emit-side-tables.md);
lazy decode and the deferred verification run there, and the test's
windows row checks identity and the lazy path only. Windows edit
rebuilds are the next item.
