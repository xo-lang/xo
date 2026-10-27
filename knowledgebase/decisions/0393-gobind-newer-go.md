# 0393: xo-gobind built with the installed Go when it is newer

Status: Proposed (Xo Lang 2, 2026-10-27; milestone 0.34.0 criterion 1,
the 0.33.0 Windows tester's bug 14)

## Context

`use go` binds a Go package with the helper xo-gobind (cmd/xo-gobind,
decision 0178 stage 6), a Go program because it reads Go's type
information: it runs the installed `go list -export` and reads the export
data it writes with `go/importer`. A release ships the helper prebuilt
(bin/xo-gobind, built by scripts/release.sh with the release machine's
Go, go1.26.1 for 0.33.0).

Go's export data format changes between Go releases, and an importer
reads only the formats its own Go knows. The 0.33.0 tester had Go 1.27
installed: go1.26's importer panics on Go 1.27's export data (`cannot
decode "strings", export data version 4 is greater than maximum
supported version 2`), so every bind failed with a Go stack trace, on
every OS. The workaround was to build the helper from the release's tag
with the installed Go and point `XO_GOBIND` at it.

Two parts: the helper must never panic on this (fixed separately: an
error naming both Go versions and the workaround), and a release should
keep binding when the installed Go is newer than the one it was built
with.

## Decision

1. A release ships the helper's Go source beside std, in the repository's
   own layout under lib/xo: `go.mod` (module github.com/xo-lang/xo, no
   requirements), `cmd/xo-gobind`, `internal/gobind` and `internal/gotool`
   (no tests; about 112 KB). The helper's only other package,
   github.com/xo-lang/xo/std (std/embed.go), is already in lib/xo/std.
2. When xo finds a prebuilt helper (beside xo, or in the release layout),
   it compares the helper's Go (`go version <helper>`) with the installed
   go command's (`go env GOVERSION`, the same command the helper will
   run). When the installed Go's language version is newer and the source
   is there, xo builds the helper once with the installed Go (`go build
   -o <cache>/xo-gobind ./cmd/xo-gobind` in lib/xo) into the user cache,
   `<user cache>/xo/gobind/<xo version>-<go version>/`, and uses that
   build from then on. Without the source (a checkout's bin, a helper on
   PATH) it uses the helper as before.
3. `XO_GOBIND` still wins over everything; a checkout still builds from
   its own cmd/xo-gobind as today.
4. A build failure is an error naming the go command, its version and
   the build output; the prebuilt helper is not tried after it (it cannot
   read the newer export data).

## Consequences

- A release keeps working when the installed Go moves ahead of the one
  the release was built with, offline, at the cost of one helper build
  (a few seconds) per xo version and Go version.
- Two go command runs (`go version`, `go env GOVERSION`) are added to a
  bind; a bind already runs `go list`. They are not on any path but a
  binding cache miss.
- The release layout gains lib/xo/go.mod, lib/xo/cmd, lib/xo/internal;
  scripts/release.sh and release-verify.sh change with it, and
  release-verify checks that the shipped source builds.
- scripts/docker-newest-go.sh (the newest Go release in a container,
  owner 2026-10-27) runs the use-go goldens and binds through the
  shipped-style helper path, so the next Go release's format change is
  caught before a tester finds it.

## Alternatives

- **`go install github.com/xo-lang/xo/cmd/xo-gobind@<tag>`** with the
  installed Go. No source to ship, but it needs the network and the
  module proxy, and an offline machine that binds only std packages
  would stop working.
- **Ship several helpers, one per supported Go.** Only covers Go
  releases that exist at release time; the tester's case was a Go newer
  than the release.
- **Read export data with golang.org/x/tools' gcexportdata.** Its newest
  version tracks Go's formats, but a helper built from it still has a
  fixed version, so a later Go breaks it the same way; it also adds a
  dependency to a module that has none.
- **Error only (part 1).** Leaves the user to build the helper by hand,
  which is the workaround the tester already needed.
