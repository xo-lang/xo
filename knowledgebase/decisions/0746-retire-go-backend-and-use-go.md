# 0746: Retire the Go backend and `use go`

Status: Accepted (owner, 2026-11-22, xw decision #283; task #721). The
stages below are the plan; each lands by its own change. Amended the
same day by the owner's two answers (xw decisions #287 and #288):
`--hybrid` goes at stage 1, `xo run --record` gets a native recorder as
a stage of its own, and stage 2 ships in 0.56.

## Context

Xo started as a compiler to Go. The Go backend (compiler/gen, with its
runtime std/rt) is what is left of that: the native backends are the
default since decisions 0248 and 0270, the Go compiler itself was
deleted by decision 0215, and std is Proven on the native backends.
`use go` (spec 4.4) lets an Xo program call a Go package; it works only
on the Go backend, through internal/gobind and cmd/xo-gobind.

Keeping the Go backend costs every std change a second implementation
or a second proof (decision 0312: the Go backend compiles the pure Xo
modules, and std/rt holds the rest in Go), a tier step, and a second
set of semantics to keep equal.

The owner's reason (2026-11-22): std is Proven, and an AI can port the
Go libraries people would reach for, which moves the ecosystem to Xo
instead of leaning on Go's. C interop (`use c`, spec 4.5) stays.

## Decision

The Go backend and `use go` are retired, in four stages. Until stage
1 lands, std work goes on as today, and no new feature is added that
only the Go backend has.

1. **std no longer needs a Go backend version.** The rule that a std
   feature works on the Go backend (decision 0312 and the goldens that
   run on `go`) is lifted. New std work targets the native backends and
   LLVM (decision 0745). `--hybrid` is removed with this stage
   (CHANGELOG Breaking: drop the flag; decision 0084 is superseded).
2. **`use go` and gobind are removed, in 0.56.** `use go` becomes an
   error with a message that names what to do: port the Go library to
   Xo, or call it over a process or network boundary (`use c` reaches
   a C library). internal/gobind, cmd/xo-gobind, the `go` effect of
   bindings and the vouch rules for Go packages leave the spec. The
   website's pages that teach `use go` change with it (xo-4c).
3. **A native recorder for `xo run --record`.** The native backends
   write the trace format std/rt/replay.go writes, so
   compiler/cli/servers/replay.xo replays native programs. It lands
   before stage 4, which removes the only recorder there is today.
4. **compiler/gen and std/rt are removed**, with `--backend=go` and
   the tier's gen step.

Each stage updates spec/core.md, CHANGELOG.md (Breaking, with what
users do) and the website's documentation in the same change.

## Inventory (origin/main, 2026-11-22; counted with git ls-files and git grep)

What goes:

| Piece | Size |
|---|---|
| std/rt (the Go backend's runtime) | 153,250 lines of Go in 360 files |
| compiler/gen (the Go emitter) | 16,978 lines of Xo |
| internal/gobind and cmd/xo-gobind | 5,859 lines of Go |
| internal/gen's goldens | 168 programs |
| std/go.xo (the values that cross the Go boundary) and the lists std/go_compiled.txt, go_optional.txt, go_proto_free.txt (which std modules the Go backend compiles, decision 0312) | 1 module, 3 lists |

Only the Go backend has today (compiler/xo main.xo, the choice of the
backend): `--hybrid` (dropped at stage 1), `xo run --record` (made
native at stage 3), and `use go` (removed at stage 2).

`use go` in this repository, outside internal/gen's own goldens (22
files there): bench/usego-workflows (aws, chi, pgx, toml, yaml, zap),
bench/perf/gobridge, std/evidence/go/stress, internal/stdtest's gofuzz
corpus, internal/selfhost's hatches test data, and cmd/xo's
go-const-called habit. Each is removed or rewritten at stage 2.

Not counted here, to be listed before the stage that needs it:

- Targets a user builds with `--backend=go` that no native backend or
  LLVM serves (Go cross compiles to every GOOS and GOARCH).
- Outside users of `use go`: it has been public since the 0.x
  releases, so stage 2 is a breaking change. The owner ships it in
  0.56 without a release of notice (xw decision #288); the CHANGELOG's
  Breaking entry, the release notes and the website say what users do.
- std modules or functions that exist only in std/rt.
- The checks that run through the Go backend: the tier's gen step
  (446 to 788 s by the lead's count), internal/gen's tests, the
  differential tests that compare `go` with the native backends
  (scripts/reduce-diff.py, progen), and what the Go backend's output
  is the oracle for.

What stays in Go: the test drivers and test infrastructure
(internal/xotool, progen, the Go tests that drive the xo binary). This
decision is about the backend and the interop, not about the language
the tests are written in.

## Consequences

- One runtime (std/native) and one set of semantics for std; a std
  change is proven on the native backends and on LLVM.
- A program that uses `use go` stops building at stage 2 (0.56). What
  its author does: port the Go package to Xo, or call it over a process
  or network boundary; use std where it has the
  module (of what bench/usego-workflows reached Go for, std has http,
  sql, sqlite, toml, yaml and log; a Postgres driver and the cloud
  SDKs are outside std), or reach a C library with `use c`.
- The differential oracle is lost with the Go backend: native against
  LLVM (decision 0745's reference) and Go's own test suites ported
  with std take its place.
- Decisions that the Go backend carries (0098, 0177, 0312, 0356, 0393,
  0412, 0415, 0416 among them) are marked superseded by this one at
  the stage that removes their subject.

## Alternatives

- Keep the Go backend as a third target: the cost above, for a backend
  that is no longer anyone's default.
- Keep `use go` on its own: it needs the Go backend, so it keeps all of
  it.
- Remove everything in one change: every breaking step at once, with
  `xo run --record` gone before its native recorder exists.
- Announce the removal of `use go` a release ahead: the owner chose
  0.56 (xw decision #288).
