# `use go` bindings land in the source tree unless XO_GOBIND_DIR is set

Status: Observed (2026-10-08, Go interop prototype; again 2026-11-15)

Symptom: after running `xo run internal/gen/testdata/golden/go_uuid`, the
golden directory held a `.xo/gobind/` tree (declaration files, adapters,
and a Go module `_mod/` with go.mod and go.sum) that `git status` would
show.

Cause: the binding cache of a `use go` import is
`<xo.mod dir or module dir>/.xo/gobind` (internal/gobind `CacheDir`), and
`xo check`, `xo build`, `xo run`, and `xo test` bind on demand. A test
data directory has no xo.mod above it inside the repository, so the
module directory itself became the cache root.

Fix: `.xo/` is in `.gitignore`. Tests that check or build Xo code with
`use go` set `XO_GOBIND_DIR` to a temporary directory
(`internal/gen/golden_test.go` TestMain, `internal/check/rules_go_test.go`,
`internal/gen/gobind_test.go`). Delete stray caches with
`find . -name .xo -type d -prune -exec rm -rf {} +`. Go and the Xo module
walk skip dot directories, so a stray cache never builds, but it does
pin a stale binding until its version header changes.

Again (2026-11-15): main's hourly `-short` failed in internal/gen
TestGoldenSQLDriverParity with "read
testdata/golden/go_jwt_mapclaims/.xo: is a directory". A `use go`
golden run outside the test harness (no `XO_GOBIND_DIR`) left the
untracked `.xo` cache in the golden, and `filepath.Glob("*.xo")` matches
it: Go's `*` matches a leading dot, so the cache directory's name is a
`*.xo` match. A test that globs source files must skip dot names and
anything not a regular file (the test now does).
