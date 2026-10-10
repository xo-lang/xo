# Test speed (2026-10-09)

Changes (commits 46840ba, 995cda7): a binary cache so identical builds
across test modes build once, more tests marked parallel, no fixed sleep
after sanitizer runs, and native builds link the C runtime from an object
cache instead of recompiling it per program. The planned per-package
timing script was dropped by the owner.

Full tests of the slow packages, darwin/arm64 (M4 Pro, 12 cores), nothing
else running, before (full suite of 2026-10-09 afternoon) and after (this
merge):

| Package | Before | After |
|---|---:|---:|
| internal/llvm | 217 s | 135 s |
| internal/gen | 203 s | 126 s |
| internal/arm64 | 177 s | 57 s |
| cmd/xo | 146 s | 67 s |
| internal/benchharness | 44 s | 24 s |
| internal/selfhost | 31 s | 14 s |
| internal/build | 32 s | 61 s (more tests since: TLS, tzdata, caches) |

Wall time for vet, the short suite, and these seven packages: 170 s
(the earlier full suite took 210 to 222 s with fewer tests).
