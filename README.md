# Xo

Xo is a small, statically typed, garbage collected language for backends and
web services, designed so AI agents can write, check, cross compile, and debug
programs with fast, structured feedback. It keeps what makes Go productive
(few keywords, one obvious way, fast builds, static binaries, a strong
standard library, a compatibility promise) and moves the bugs tests usually
miss into the compiler.

Status: experimental. The spec is at draft 0.7 and the P1 toolchain compiles
Xo to Go. Expect breaking changes.

```
use std/http

pub struct User {
  pub id: Int
  pub name: Str
  pub email: Str?
} derive Json

pub enum ApiErr {
  BadRequest(msg: Str)
  NotFound(id: Int)
}

fn (e: ApiErr) status() -> Int {
  match e {
    BadRequest(_) => 400
    NotFound(_) => 404
  }
}

fn (e: ApiErr) display() -> Str {
  match e {
    BadRequest(msg) => msg
    NotFound(id) => "user ${id} not found"
  }
}

struct Store { users: Mutex[Map[Int, User]] }

fn (s: Store) get(id: Int) -> User ! ApiErr
  requires id > 0 else ApiErr.BadRequest(msg: "id must be positive")
{
  s.users.with(fn(m) { m.get(id) }).or_err(ApiErr.NotFound(id: id))?
}

fn main(os: Os) ! {
  let store = Store{users: Mutex.new({1: User{id: 1, name: "Ada", email: None}})}
  var r = http.router()
  r.handle("GET /users/{id:Int}", fn(req) -> http.Response ! ApiErr {
    http.json(200, store.get(req.param_int("id"))?)
  })
  http.server(os.net.listen(":8080")?, r).serve()?
}
```

## What is different from Go

| Problem in Go | Xo |
|---|---|
| Ignored errors compile | Unhandled `Result` is a compile error; discarding needs `.ignore_err("reason")` |
| nil panics | No nil: optionals (`T?`), and `Map` reads by index are rejected |
| Missing `switch` cases | Sum types with exhaustive `match` |
| Data races | Value semantics: no shared mutable values outside `Mutex` and `Atomic` |
| Goroutine leaks, `ctx` plumbing | Structured concurrency: no task outlives its scope; cancellation flows down |
| Any package can touch files or the network | Effects in signatures (`uses net, fs.read`) and capabilities passed from `main` |
| Broken code hidden behind build tags | Every `match target.os` arm is type checked for every target |
| Flaky concurrency tests | Deterministic scheduler; `xo test --explore=N` searches interleavings |
| Hard to reproduce failures | Contract fuzzing writes regression tests; record and replay (in progress) |

## Install

`xo` is written in Xo (the self hosted toolchain, `selfhost/xo`; decision
0178 switch point B). A release archive (`scripts/release.sh`, for
darwin/arm64, linux/arm64, and linux/amd64) unpacks to

```
xo-<version>-<os>-<arch>/bin/xo          the toolchain
xo-<version>-<os>-<arch>/bin/xo-gobind   the Go binder helper (`use go`)
xo-<version>-<os>-<arch>/lib/xo/         std, the prebuilt C runtime
```

Keep the layout: `bin/xo` finds std and the runtime in `../lib/xo`
(or set `XO_STD`), so a copy of `bin/xo` alone does not work. Put the
`bin` directory on PATH, or link `bin/xo` from a PATH directory
(symbolic links are followed).

From a checkout, an existing `xo` builds the next one: there is no Go
implementation of the compiler any more (decision 0215). The seed is a
previous `xo`: a release's `bin/xo`, an earlier build, or a prebuilt
seed (decision 0216: release N is built by release N-1; the project's
prebuilt seeds are kept privately and are not public downloads). One
command does it:

```
XO_SEED=/path/to/xo go run ./scripts/xobuild   # prints the path of bin/xo
go run ./scripts/xobuild                       # with an earlier build or xo on PATH
```

It builds `bin/xo` (git ignored) from the checkout's `selfhost/xo` when
its sources changed, with the arm64 backend on an arm64 host (Apple
Silicon, arm64 Linux) and the LLVM backend on an x86-64 host (clang
needed). The seed order (`$XO_SEED`, the checkout's earlier `bin/xo`,
the newest earlier build in `<user cache>/xo/checkout`, `xo` on PATH,
then the project's prebuilt seed) is internal/xotool's; when none is
found, it stops with an error instead of building one.

With no `xo` at all, build from source (opt in, slow):
`go run ./scripts/xobuild --from-source` builds the `go-compiler-final`
tag (the last commit with the Go compiler; Go 1.26 or later), then the
hops in `scripts/seed-chain.txt`, then the checkout (25 to 37 minutes on
a 16 vCPU machine as of 2026-10-13, decision 0216). `bin/xo` finds
`std` at `../std`, as a release's finds `lib/xo/std`. After that, any Xo
`xo` rebuilds itself without Go (`scripts/bootstrap-xo.sh`).

The module path is `github.com/xo-lang/xo` (local name; see
`knowledgebase/infrastructure/toolchain.md`).

The default backend is native (decision 0248): arm64 on arm64 hosts,
LLVM (which needs clang) on x86_64 hosts. `--backend=go` builds through
the Go toolchain, and programs with `use go` need it. The arm64 backend
assembles and links with Xo's own assembler and linkers on macOS and
for Linux (no C toolchain); `--ld=cc` uses the system's. The arm64 backend's C runtime (std/llrt)
comes prebuilt in a release archive (`lib/xo/rtpack`, built by
`scripts/release.sh`); an xo built from a checkout compiles it on the
first native build and caches it (decision 0157).

## Commands

| Command | Purpose |
|---|---|
| `xo check [--json] <dirs>` | Type check; JSON diagnostics with stable codes and fixes |
| `xo build [--targets=all] [--backend=go\|llvm] <dir>` | Static binaries for linux, darwin, windows, wasi |
| `xo run <dir>` | Build and run for the host |
| `xo test [--seed=N] [--explore=N] [--race] <dir>` | Tests, property tests, contract fuzzing, deterministic scheduling |
| `xo fmt [--check]` | Canonical formatting, no options |
| `xo vet` | Advisory checks |
| `xo query <type\|def\|callers\|effects\|holes\|api>` | Semantic queries for tools and agents |
| `xo edit <rename\|add-field\|change-sig>` | Semantic edits through resolved symbols, rechecked before writing |
| `xo doc`, `xo explain <code>` | Documentation and diagnostic explanations |
| `xo bind <go package>` | Bindings for `use go "<package>"` |
| `xo bind --c <header>` | Bindings for `use c "<header>"` (native builds, `--no-gc`) |
| `xo dump <pid>` | Task tree of a running program |

## Project layout

| Path | Contents |
|---|---|
| `spec/core.md`, `spec/std.md` | Language specification and standard library (source of truth) |
| `cmd/xo` | The CLI |
| `internal/lexer`, `parser`, `ast` | Front end |
| `internal/check` | Type, effect, mutation, and concurrency checking |
| `internal/gen`, `internal/build` | Code generation to Go, build driver |
| `internal/llvm`, `std/llrt` | LLVM backend prototype and its C runtime |
| `internal/gobind` | Go package bindings |
| `std/*.xo`, `std/rt` | Standard library declarations and the Go runtime |
| `selfhost/` | Compiler components written in Xo (`selfhost/lexer`), checked against the Go ones by `internal/selfhost` |
| `bench/` | Benchmarks: tasks, Go and Xo solutions, planted bugs, results |
| `examples/` | Realistic programs; `notes-service`: HTTP JSON, std/sql, auth middleware, background job, graceful shutdown |
| `knowledgebase/` | Decisions (ADRs), gotchas, infrastructure notes |

## Evidence so far

All measured in `bench/results/`:

- Writing code: agents passed their own tests on the first try about as often
  in Xo as in Go, with Xo programs about half the size.
- Planted safety bugs (ignored errors, nil, missing cases, races, leaks, lost
  updates): Xo prevented or caught 23 to 29 of 30 across two independent bug
  authors, against 13 for Go with strict analyzers and 9 for plain Go. Logic
  bugs were caught at the same rate in both.
- Performance: with monomorphization, Xo beats idiomatic Go generics on 5 of 6
  generic workloads; the LLVM prototype is at or near Go on compute. Request
  throughput is still about 1.7x behind Go.

## Development

```
go test ./...
go vet ./...
GOBIN=$PWD/.tools/bin go install honnef.co/go/tools/cmd/staticcheck@latest
.tools/bin/staticcheck ./...
```

The Go benchmark arms also use errcheck, nilaway, and exhaustive in
`.tools/bin` (see `bench/go/strict-check.sh`). LLVM builds need `clang`;
arm64 builds need `cc` (see Install).

Read `CLAUDE.md` before contributing (safety rules and the knowledgebase
workflow). Design decisions live in `knowledgebase/decisions/`.
