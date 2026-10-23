# 0356: A third-party `use go` needs a `require go` pin

Status: Accepted (owner, 2026-10-23; option A of Xo Lang 2's proposal).

## Context

Intel field report 0.26.0, round 2: a `use go "github.com/google/uuid"`
with no `require go` line in xo.mod builds with whatever version Go's
module resolution picks (the latest), and the version is recorded only in
the binding cache under the gitignored `.xo/gobind`. Two machines, or one
machine on two days, can build different code from the same commit, and
xo.lock (core 4.3) has no line for Go modules, so XO1304 and XO1305 never
see them. The effect lock (decision 0156) covers Xo dependencies only.

## Decision

1. A `use go` of a package outside Go's standard library (a binding whose
   header is not `go-standard: true`) in a module whose xo.mod has no
   `require go` line for the Go module providing it is error `XO1319`:
   "Go module M has no `require go` line in xo.mod: builds would follow
   its latest version". The fix inserts `require go M vX.Y.Z` into
   xo.mod with the version the binding resolved, so `xo fix` (or an
   agent applying the fix) pins it in one step. `xo mod tidy` adds the
   missing lines too.
2. Go standard library packages (`regexp`, `strconv`) need no line: the
   Go toolchain's version decides them, and the build information
   records it.
3. xo.lock records each pinned Go module: `go <module> <version>
   h1:<hash>` and `go <module> <version>/go.mod h1:<hash>`, Go's own
   go.sum hashes for the module zip and its go.mod (the binding
   workspace's go.sum, checked by the Go command against sum.golang.org
   unless the user's GOFLAGS say otherwise). Every check compares them
   with the binding's go.sum: a different hash is `XO1304`, a pinned Go
   module with no line is `XO1305`; `xo mod tidy` and `xo get` write
   them. A workspace override is never written, as for Xo modules.
4. Stages: (a) XO1319 with its fix, tidy adding the pins, spec 4.4 and
   10; (b) the xo.lock lines and their checks, spec 4.3.

## Consequences

- Breaking: a program with a third-party `use go` and no pin stops
  checking until the line is added (`xo fix` adds it). The CHANGELOG
  says so.
- A pinned build fetches exactly that version; an upgrade is an edit of
  the `require go` line (or `xo get go M@vX`), visible in review.
- The Go backend is the only backend for `use go`, so native builds are
  unaffected.

## Alternatives

- B, lock the first resolution in xo.lock and keep floating until then:
  reproducible once xo.lock is committed, but an unpinned build silently
  picks "latest" first and xo.mod does not say what runs.
- C, write `require go` into xo.mod automatically on the first build, as
  Go 1.15 and earlier did with go.mod: Go made `-mod=readonly` the
  default in 1.16 because builds silently edited go.mod
  (https://go.dev/ref/mod).
- A warning for one release before the error: the owner chose the error.
