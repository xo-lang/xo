# Xo

Xo is a small, statically typed, garbage collected language for backends and
web services, designed so AI agents can write, check, cross compile, and debug
programs with fast, structured feedback. It keeps what makes Go productive
(few keywords, one obvious way, fast builds, static binaries, a strong
standard library, a compatibility promise) and moves the bugs tests usually
miss into the compiler.

Status: experimental, toolchain 0.32.0. The compiler and the `xo`
toolchain are written in Xo and compile to native code (arm64, amd64,
LLVM), Go, and WebAssembly. Xo stays on 0.x and expect breaking changes until
1.0.

The repository is mostly Xo: about 20 MB of Xo in the compiler, toolchain
and standard library (685 files), and about 52 MB counting tests and
benchmarks, against about 6 MB of Go (the Go backend's runtime, the test
drivers and benchmark twins). GitHub's language bar does not recognize
`.xo` files, so it shows only the Go.

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

The [latest release](https://github.com/xo-lang/xo/releases/latest) has
archives for macOS (Apple silicon and Intel, signed and notarized), Linux
(arm64 and x86-64) and Windows (x86-64, Authenticode signed), with a
`SHA256SUMS` file. Check the archive against it, unpack it, and put its
`bin` directory on your `PATH`; `bin/xo` finds the standard library at
`../lib/xo`, so keep the directory together. The steps for each platform
are in [Getting started](https://xolang.dev/docs/getting-started/#install)
on [xolang.dev](https://xolang.dev), which also has the language tour,
the specification and the release notes.

## Building from source

The compiler is written in Xo, so an existing `xo` builds the next one
(decision 0215 removed the Go implementation; decision 0216: release N is
built by release N-1). An installed release usually builds this
checkout; one older than the checkout's minor version may not:

```
XO_SEED=/path/to/xo go run ./scripts/xobuild   # prints the path of bin/xo
```

With no `xo` at all, the first build starts from Go:

```
go run ./scripts/xobuild --from-source   # Go 1.26 or later; prints the path of bin/xo
```

It builds the `go-compiler-final` tag (the last commit with the Go
compiler), then the hops in `scripts/seed-chain.txt`, then this
checkout's `compiler/xo` into `bin/xo` (git ignored): 25 to 37 minutes on
a 16 vCPU machine (2026-10-13). After that, any `xo` rebuilds the next
one without Go:

```
XO_SEED=/path/to/xo go run ./scripts/xobuild   # or with bin/xo or xo on PATH
scripts/bootstrap-xo.sh                         # the self rebuild fixed point
```

`bin/xo` finds the standard library at `../std`; a release layout
(`scripts/release.sh`) puts it in `lib/xo/std` beside `bin/`, and
`XO_STD` overrides both.

## Backends

The default backend is native (decisions 0248 and 0270): arm64 on arm64
hosts (Apple Silicon, arm64 Linux) and amd64 on x86-64 hosts (macOS,
Linux, Windows), with Xo's own assemblers and linkers; `--race` stays native where a
ThreadSanitizer runtime ships with the release (darwin, linux, and
windows/amd64) and uses `--backend=llvm` (clang needed) elsewhere. `--backend=go`
builds through the Go toolchain, which `use go` bindings need, and
`--backend=wasm` builds a WASI preview1 module. `xo build --targets=all`
cross compiles natively for linux and darwin (amd64 and arm64),
windows/amd64, and wasi; `--backend=go --targets=all` adds
windows/arm64.

## Commands

| Command | Purpose |
|---|---|
| `xo check [--json] <dirs>` | Type check; JSON diagnostics with stable codes and machine fixes |
| `xo build`, `xo run` | Static binaries for the host or `--targets`; build and run |
| `xo test [--seed=N] [--explore=N]` | Tests, property tests, contract fuzzing, deterministic scheduling |
| `xo fmt`, `xo fix`, `xo vet` | Canonical formatting, machine fixes, advisory checks |
| `xo query`, `xo edit` | Semantic queries (type, def, callers, effects, holes, api) and edits (rename, add-field, change-sig) |
| `xo doc`, `xo explain <code>` | Documentation and diagnostic explanations |
| `xo init`, `xo get`, `xo mod`, `xo vendor`, `xo publish`, `xo audit` | Modules, versions, and supply chain checks |
| `xo bind` | Bindings for `use go` packages and `use c` headers |
| `xo dump`, `xo replay`, `xo prof`, `xo trace` | Task trees, replay of recorded runs, profiles, execution traces |
| `xo lsp`, `xo mcp`, `xo serve` | Language server, Model Context Protocol server, warm checker |

`xo help` lists them all; `xo <command> -h` gives the flags.

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
| `editors/` | Editor support (VS Code) |

## Evidence

Planted safety bugs (ignored errors, nil, missing cases, races, leaks,
lost updates), 30 per language: Xo prevented or caught 23 to 29, against
13 for Go with strict analyzers and 9 for plain Go
([planted-xo-2026-10-07.md](bench/results/planted-xo-2026-10-07.md));
with Codex, a second model family, as the author, 22 to 27 against 17
for strict Go and 12 for plain Go
([codex-2026-10-09.md](bench/results/codex-2026-10-09.md)). Against Go on the
same machine, the 0.26.0 release ran 14 of 20 benchmark workloads faster
and all 20 in less peak memory, with smaller binaries
([release-vs-go-v0.26.0.md](bench/results/release-vs-go-v0.26.0.md)).
Performance and code size results for each backend are indexed in
[bench/results/README.md](bench/results/README.md).

## Development

```
go test -short ./...   # about 10 s once bin/xo is built
go vet ./...
```

The Go tests drive the `xo` binary (`$XO_CLI`, else the checkout's
`bin/xo`, rebuilt when `compiler/` or std changes). Tiers, the fixed
point, and the full suites are described in `CLAUDE.md` and
`knowledgebase/infrastructure/toolchain.md`.

## Contributing and license

Issues are welcome; code arrives as proposals, see
[CONTRIBUTING.md](CONTRIBUTING.md) and, for vulnerabilities,
[SECURITY.md](SECURITY.md). Xo is MIT licensed ([LICENSE](LICENSE)); the
standard library is MIT-0 ([std/LICENSE](std/LICENSE)), so programs
built with Xo need not carry Xo's notice, and code from Go keeps its BSD
license ([THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES)). Everyone taking part
follows the [code of conduct](CODE_OF_CONDUCT.md).
