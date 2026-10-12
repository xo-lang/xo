# `go vet` in a build directory recompiles rt for every program

Status: Observed (2026-10-11)

Symptom: the default Go build cache (`~/Library/Caches/go-build`) grew
from 28 MB to 111 GB between 2026-10-10 23:00 and 2026-10-11 03:00, and
the disk filled. Most of it was about 1,700 identical-sized (24.5 MB)
archives of package `rt`, each with a different build ID and a source
path `.../build/<hash>/rt`.

Cause: `TestGeneratedGoVets`, `TestGeneratedGoVetsLarge` and
`TestArithProperty` (the nightly run takes new seeds every day) ran
`go vet` in each program's build directory with the default `GOCACHE`
and no `-trimpath`. Every build directory has its own copy of `rt`, and
without `-trimpath` Go puts the package directory in the cache key, so
each program compiled `rt` again. `xo build` itself was not affected: it
passes `-trimpath` and uses the capped Xo cache (`build.GoCacheDir()`).

Fix: the test helpers run go with `goVetEnv()`
(`internal/gen/vet_test.go`): `GOFLAGS=-trimpath`, `GOWORK=off`,
`GOCACHE=build.GoCacheDir()`. With it, the 121 programs of
`TestGeneratedGoVets` write 14 `rt` archives (one per mode and flag set)
into a 1.1 GB cache. Any test that runs the go command in a build
directory should use the same environment.

Update (2026-10-12, decision 0176): the default cache was at 19 GB
again. 235 of its files were this gotcha's 24.8 MB `rt` archives from
before the fix; two more go commands in tests still used the default
cache (`go build` in internal/gen reducediff_test.go, and `go tool asm`
and `go tool objdump` in internal/arm64 gosyntax_test.go; Go 1.25 and
later build objdump on first use). Both now use `build.GoEnv()` or
`gotool.CacheEnv()` (`goVetEnv()` is `build.GoEnv()`), and
`TestGoCommandsUseXoCache` (internal/build) fails on any new
`exec.Command("go", ...)` that builds without one. What still grows the
default cache is the repo's own `go test` per worktree path
(toolchain.md, "Go build cache").
