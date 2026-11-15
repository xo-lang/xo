# Xo

Xo is a small, statically typed, garbage collected language for backends and
web services, designed so AI agents can write, check, cross compile, and debug
programs with fast, structured feedback. It keeps what makes Go productive
(few keywords, one obvious way, fast builds, static binaries, a strong
standard library, a compatibility promise) and moves the bugs tests usually
miss into the compiler.

Status: experimental, 0.x; expect breaking changes until 1.0. The
[latest release](https://github.com/xo-lang/xo/releases/latest) has the
current version. The compiler and the `xo` toolchain are written in Xo, so
the repository is mostly Xo; GitHub's language bar shows only the Go.

Xo is built almost entirely by AI agents working under one maintainer,
and the full git history is kept to show it: every design decision is an
ADR in `knowledgebase/decisions/`, and every surprise a gotcha in
`knowledgebase/gotchas/`. This repository is a mirror of the development
repository, published with private values (account IDs, addresses, local
paths) replaced; see [CONTRIBUTING.md](CONTRIBUTING.md).

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
| Hard to reproduce failures | Contract fuzzing writes regression tests; `xo run --record` and `xo replay` |

## Install

Download an archive from
[Releases](https://github.com/xo-lang/xo/releases/latest) and follow
[Getting started](https://xolang.dev/docs/getting-started/#install) on
[xolang.dev](https://xolang.dev), which also has the language tour, the
specification and the release notes.

## Building from source

The compiler is written in Xo, so an existing `xo` builds the next one:

```
XO_SEED=/path/to/xo go run ./scripts/xobuild   # prints the path of bin/xo
go run ./scripts/xobuild --from-source         # no xo yet: starts from Go 1.26 or later
scripts/bootstrap-xo.sh                         # the self rebuild fixed point
```

## Backends

Builds are native by default (arm64 and amd64, with Xo's own assemblers
and linkers); `--backend=llvm`, `--backend=go` and `--backend=wasm` select
the LLVM, Go and WebAssembly backends. The targets and flags are in the
[documentation](https://xolang.dev/docs/).

## Commands

| Command | Purpose |
|---|---|
| `xo check [--json]` | Type check; JSON diagnostics with stable codes and machine fixes |
| `xo build`, `xo run` | Static binaries for the host or `--targets`; build and run |
| `xo test` | Tests, property tests, contract fuzzing, deterministic scheduling |
| `xo fmt`, `xo fix`, `xo vet` | Formatting, machine fixes, advisory checks |
| `xo get`, `xo mod` | Modules and versions |
| `xo debug` | Debugger for native programs (`--dap` for editors) |

`xo help` lists all; `xo <command> -h` gives the flags.

## Project layout

| Path | Contents |
|---|---|
| `spec/core.md`, `spec/std.md` | The language specification and standard library surface (source of truth) |
| `compiler/` | The compiler and toolchain in Xo: `lexer`, `parser`, `check`, `ssa`, backends (`arm64`, `llvm`, `gen` for Go, `wasm`), `macho` (assembler and linkers), `cli`, `xo` (the driver) |
| `std/` | The standard library: declarations, `std/native` and `std/llrt` (native runtime), `std/rt` (the Go backend's runtime) |
| `cmd/xo`, `internal/`, `test/` | Go test drivers that run the `xo` binary, Go support code (`xotool`, `gobind`, `progen`), compiler test inputs |
| `cmd/xo-gobind` | The Go binder helper for `use go` |
| `bench/` | Benchmark tasks, Go and Xo solutions, planted bugs, results |
| `examples/` | Realistic programs (`notes-service`: HTTP JSON, std/sql, auth middleware, background job, graceful shutdown) |
| `knowledgebase/` | Decisions (ADRs), gotchas, infrastructure notes |

Editor support: the VS Code extension (syntax, `xo lsp`, `xo debug --dap`)
lives in its own repository, [xo-lang/vscode](https://github.com/xo-lang/vscode)
(decision 0516).

## Evidence

Benchmark, safety and code size results are indexed in
[bench/results/README.md](bench/results/README.md) and summarized on
[xolang.dev](https://xolang.dev).

## Development

```
go test -short ./...
go vet ./...
```

The Go tests drive the `xo` binary (`$XO_CLI`, else `bin/xo`); the tiers
are in `knowledgebase/infrastructure/toolchain.md`.

## Contributing and license

Issues are welcome; code arrives as proposals, see
[CONTRIBUTING.md](CONTRIBUTING.md) and, for vulnerabilities,
[SECURITY.md](SECURITY.md). Xo is MIT licensed ([LICENSE](LICENSE)); the
standard library is MIT-0 ([std/LICENSE](std/LICENSE)), so programs
built with Xo need not carry Xo's notice, and code from Go keeps its BSD
license ([THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES)). Everyone taking part
follows the [code of conduct](CODE_OF_CONDUCT.md).
