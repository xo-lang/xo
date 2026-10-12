# 0176: Supported Go versions for the Go backend, and every go command on Xo's cache

Status: Accepted (2026-10-12), implemented: `internal/gotool`,
`internal/build` (XO1502, `GoEnv`, `TestGoCommandsUseXoCache`),
`internal/gobind` (workspace go line), `internal/gen` (`Options.GoLine`),
`scripts/go-version-check.sh`.

## Context

The Go backend runs the go command (`$XO_GO`, else `go` on PATH) on the
generated program and on `use go` binding workspaces. Three things were
fixed to the development host's Go 1.26:

- the generated go.mod (internal/gen) and the binding workspace go.mod
  (internal/gobind) both said `go 1.26`. With an older go command and the
  default `GOTOOLCHAIN=auto`, a go.mod line newer than the go command
  makes it download that Go; with `GOTOOLCHAIN=local` it fails with
  "go.mod requires go >= 1.26". Neither tells the user what Xo needs.
- nothing stated or checked the oldest Go that builds generated programs
  and std/rt.
- there was no procedure for a new Go release, although std/rt reaches
  into the Go runtime (the `runtime.timeSleepUntil` linkname of decision
  0073) and binds the standard library through `go list -export`.

Separately, the default Go cache (`~/Library/Caches/go-build`) reached
19 GB on 2026-10-11 (20,461,608 KiB on 2026-10-12), after the 111 GB
episode of gotchas/go-vet-recompiles-rt-per-build-dir.md.

## Decision

1. **Minimum Go: 1.24** (`gotool.MinGo`). Method: std/rt (without tests)
   copied into a module whose go.mod names the candidate version, then
   `go vet -stdversion` (reports standard library symbols newer than the
   go line) and `go build` (reports language features newer than it).
   At `go 1.23`, vet reports `pbkdf2.Key` (rt/crypto.go:68, 101),
   `maphash.Comparable` (rt/gointerop.go:421, 441), and `http.Protocols`
   (rt/http_net.go:198, 366), all Go 1.24. At `go 1.24`, rt vets clean
   for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64,
   windows/amd64, and wasip1/wasm, each with no tags and with
   `xo_gotimers`, `xo_pure`, and `xo_nohttp`
   (`scripts/go-version-check.sh rt-min`, PASS with go1.26.1 on
   2026-10-12). The generated Go of the golden programs is held to the
   same line by `XO_GO_LINE=1.24` with TestGeneratedGoVets
   (`go-version-check.sh gen-min`).
2. **Check once per process.** `gotool.Version` runs `go env GOVERSION`
   once per go command path per process (in the temporary directory, so
   a go.mod in the working directory cannot switch toolchains;
   `GOTOOLCHAIN` in the environment still applies). `build.Build` checks
   it for the Go backend (`xo build`, `xo run`, `xo test`, and everything
   built on them) before the type check, and a go command older than
   MinGo fails the build with **XO1502** (error, build), naming the go
   command and its version and suggesting a newer Go, `XO_GO`, or a
   native backend. A version that cannot be read is not XO1502: the
   build then reports the go command's own failure. The native backends
   do not ask.
3. **Derived go lines.** Xo writes `go <line>` with
   `line = min(installed language version, gotool.TestedGo)`
   (`gotool.Line`): the generated go.mod (`gen.Options.GoLine`, set by
   internal/build) and a new binding workspace go.mod
   (`gobind.workspaceMod`). Xo never asks the go command for a newer Go
   than itself, so nothing is downloaded behind the user's back, and a
   newer Go keeps the tested language semantics and GODEBUG defaults
   until the suite has passed on it. `TestedGo` is 1.26. `XO_GO_LINE`
   overrides the line (for the version check script). `go get` still
   raises the workspace line when a bound module needs it.
4. **New Go releases.** Procedure (toolchain.md, "Go versions"): run
   `GOTOOLCHAIN=goX.Y[rcN] scripts/go-version-check.sh`. Its steps:
   `version`; `unit` (`go test -short` over the repo); `rt-min` and
   `gen-min` (the minimum still holds); `goldens` (TestGolden,
   TestGoldenNoMono, TestGeneratedGoVets, and the Go timers link tests,
   at the new language version through `XO_GO_LINE`); `usego`
   (TestGoBindingSignatures, TestReplayGoCalls, TestReplayGoCallback,
   internal/gobind, cmd/xo TestBind, and the checker's TestGoInterop,
   with bindings regenerated in a fresh `XO_GOBIND_DIR`). It downloads
   nothing unless `--download` is given (`GOPROXY=off` otherwise, so
   only a toolchain already in the module cache is used), and keeps its
   Go cache, Xo Go cache, and binding cache in a temporary directory it
   removes. When it passes, raise `TestedGo` (and the repo's go.mod if
   the compiler should use the new Go), update toolchain.md, and add a
   CHANGELOG line. A failure is fixed forward (a gotcha and a test),
   and the release is not adopted until the script passes. Raising
   `MinGo` needs a new decision.
5. **Every go command on Xo's cache.** Audit of 2026-10-12 (all
   `exec.Command` and `exec.CommandContext` calls and every script):

   | Where | go command | Cache |
   | --- | --- | --- |
   | internal/build `goBuildRun` | `go build` of every program | Xo cache, shared lock (unchanged) |
   | internal/gobind `runGo` | `go list`, `go get`, `go mod` | Xo cache (now via `gotool.CacheEnv`) |
   | internal/build tzdata, bincache, gotool | `go env` | reads no build cache (exempt) |
   | internal/gen vet_test, hybrid_test | `go vet` in build dirs | Xo cache (`goVetEnv` = `build.GoEnv`) |
   | internal/gen reducediff_test | `go build` of fakexo and cmd/xo | **was default**, now Xo cache |
   | internal/arm64 gosyntax_test | `go tool asm`, `go tool objdump` | **was default** (objdump is built on first use since Go 1.25), now Xo cache |
   | internal/gen golden_test | `go mod download` | module cache only; now Xo env anyway |
   | internal/gen build, replay, list_model tests | `go env CGO_ENABLED` | exempt |
   | scripts/reduce-diff.py | via xo | temporary XO_GOCACHE (unchanged) |
   | scripts/linux-test.sh | `go test` in a container | container volume `/cache/go-build` |
   | scripts/go-version-check.sh | `go test`, `go vet` | temporary GOCACHE, removed at the end |
   | scripts/release.sh, selfhost-switchover.sh, bench/size, bench/perf/*.sh | `go build` of cmd/xo or of one Go comparison program | default cache: see below |
   | bench/nightly.sh, bench/go/strict-check.sh, developers' `go test` | the repo's own tests | default cache: see below |

   `build.GoEnv()` (GOWORK=off, GOFLAGS=-trimpath, GOCACHE=Xo's) is the
   environment for go commands outside a build. **TestGoCommandsUseXoCache**
   (internal/build) parses every Go file of the repo and fails when an
   `exec.Command` or `CommandContext` of `"go"`, `goTool()`, or
   `gotool.Path()` with a subcommand other than `env` or `version` is in
   a function that does not set GOCACHE through `GoEnv()`, `goVetEnv()`,
   `CacheEnv()`, or a `"GOCACHE=` literal; an intended exception says why
   in a `//xo:default-gocache` comment. It found the three call sites
   above on its first run.

   The go commands that stay on the default cache compile the repo
   itself or one standalone binary, and run outside Xo's cache lock: on
   Xo's cache, a trim that holds the lock could delete an entry such a
   command is linking (gotchas/go-cache-trim-races-builds.md). Go trims
   the default cache itself (entries unused for 5 days).

   Attribution of the 19 GB (files over 10 MB modified in the last 2
   days, read with `strings`): 235 files of 24,787,536 bytes (5.8 GB),
   package `rt` compiled from `xo-gen-test-*/build/<hash>/rt`, written on
   2026-10-11 around 12:00, before commit bc58195 (12:09) put the gen
   tests' `go vet` on Xo's cache. Of the 46 files over 20 MB written
   after 16:00 that day, 31 are the repo's own `std/rt` package (and its
   test variant) compiled by `go test` from a worktree path
   (`.claude/worktrees/agent-*/std/rt`, 34 to 45 MB each): every
   worktree has its own path, and without `-trimpath` the path is part
   of the cache key, so each worktree compiles the repo again. 5 name
   Xo build directories or temporary test directories, 10 carry no
   path.

## Consequences

- An old Go fails fast with a stable code instead of a toolchain
  download or a go.mod error, and an agent can read `xo explain XO1502`.
- A Go newer than TestedGo builds with the tested language semantics;
  adopting it is an explicit, tested step.
- Generated programs built with an installed Go between 1.24 and 1.25
  get that release's GODEBUG defaults (for example no container aware
  GOMAXPROCS before 1.25), as a Go project with that go line would.
- The binding cache (`.xo/gobind`) is not keyed by the Go version: an
  existing workspace keeps its go line and declarations after a Go
  upgrade until the binding is regenerated.
- The remaining growth of the default cache is the repo's own tests
  across worktrees, which Xo's cache cannot take safely.

## Alternatives

- Keep `go 1.26` and require Go 1.26: simplest, but refuses Go 1.24 and
  1.25, which build everything, and still downloads with an older Go.
- Write MinGo as the go line always: every program would get Go 1.24
  semantics and GODEBUG defaults even on a newer Go.
- Use the installed version without the TestedGo cap: a new Go would
  change semantics before the suite has run on it.
- Put the repo's own `go test` on Xo's cache: it holds no cache lock, so
  a concurrent trim could delete entries mid link; giving it the lock
  needs a wrapper around every `go test`.

## Next steps

1. Run the procedure on the Go already in the module cache:
   `GOTOOLCHAIN=go1.27.2 scripts/go-version-check.sh` (no download;
   toolchain@v0.0.1-go1.27.2 is present), then raise TestedGo if it
   passes.
2. Share the repo's test compiles across worktrees with `-trimpath`
   (the worktree path then leaves the cache key). First result
   (2026-10-12): `GOFLAGS=-trimpath go test -short -count=1 ./std/rt`
   passes, writing 168 MB into an empty temporary GOCACHE. Next: the
   same over the repo, `GOFLAGS=-trimpath go test -short $(go list
   ./... | grep -v /runs/)`; if it passes, set `GOFLAGS=-trimpath` in
   bench/nightly.sh and the agents' heavy runner.
3. Key `use go` bindings by the Go language version (a `go-version:`
   header in the declaration file, checked by `gobind.Load`) so a Go
   upgrade regenerates them.
