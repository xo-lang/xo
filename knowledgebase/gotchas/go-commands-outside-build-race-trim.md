# Go commands outside an Xo build lost cache entries to a concurrent trim

Status: Observed (2026-10-13, batch 6 full run on main ad945424, EC2
r8g.8xlarge, every package in parallel; reproduced locally by
`TestGoCommandSurvivesTrim` in internal/build)

Symptom: `internal/gen TestGeneratedGoVetsLarge/selfhost_xo` failed with
`go vet` reporting `could not import strings (open
/home/ec2-user/.cache/xo/gocache/48/48c7...-d: no such file or
directory)` and `could not import xoprog/rt`, then a cascade of bogus
`missing return` errors (the type checker runs on without the imports).
Nothing was wrong with the generated code.

Cause: the Xo Go cache trim (`internal/build/gocache.go`, run after
every Xo `go build` once the cache is over its 8 GiB cap) takes the
cache lock exclusively and then removes any entry, recent ones included
(`go-cache-trim-races-builds.md`). Only Xo builds (`goBuildRun`) held
the shared side of that lock. Every other go command on the cache (the
test suites' `go vet` through `goVetEnv()`, `go build` of helper tools,
`go tool asm`, the Go binder's `go list`) ran unlocked, so a trim from
another test binary removed the export data of `strings` after go vet
had found it in the cache and before the vet tool read it. A full run
with every package in parallel fills the cap within minutes, so trims
are frequent.

Fix (2026-10-13): the lock moved to `internal/gotool`
(`cachelock_unix.go`); every go command on the cache holds
`gotool.HoldCache()` while it runs, and `TestGoCommandsUseXoCache`
fails on a go command routed to the Xo cache whose function does not
call `HoldCache()` (or explain itself with `//xo:default-gocache`).
`TestGoCommandSurvivesTrim` warms a one byte capped cache, then trims it
during a `go vet`; without the lock it fails with the same `could not
import fmt (open .../-d: no such file or directory)`, with it it passes.
On EC2 (r8g.8xlarge, 2026-10-13, instance i-EXAMPLE04) a
stress with `XO_GOCACHE_MAX=300000000` running internal/gen's
`TestGeneratedGoVets`, the -short suites of cmd/xo, internal/build,
internal/gen, internal/selfhost, and `TestGoCommandSurvivesTrim` (count
3) side by side failed on main 24982a6a with `could not import
xoprog/rt` and `could not import os (open .../-d: no such file or
directory)` in `TestGeneratedGoVetsLarge`, and passed with the fix
(970f0b5d), with no `could not import` in any log. (`TestHybridPreemption`
failed in both runs with "no GC ran during the loop", unrelated.)
Never hold the lock around something that itself runs an Xo build: a
trim waiting at the gate would deadlock with the nested shared lock.

Still open: the self-hosted driver's Go backend (`go_build` in
`selfhost/xo/gobackend.xo`) runs go build on the same cache without the
lock, because Xo's `fs.lock` is exclusive only. It needs a shared lock
in std (`fs.lock_shared` or similar) or a trim that knows about xo1.
