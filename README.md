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

Requires Go 1.26 or later. Build from a checkout:

```
go build -o .tools/bin/xo ./cmd/xo
```

The module path is `github.com/xo-lang/xo` (local name; see
`knowledgebase/infrastructure/toolchain.md`).

The Go backend (the default) needs the Go toolchain. The native backends
need a C toolchain: `--backend=llvm` needs clang, and `--backend=arm64`
needs a C compiler driver (`cc`: clang or gcc) to assemble the generated
code and run the system linker. The arm64 backend's C runtime (std/llrt)
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
