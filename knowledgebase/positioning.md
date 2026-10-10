# Why Xo: advantages over and alongside Go

Last verified: 2026-10-10 (toolchain v0.9.0). Numbers come from the
results files linked below; update them when a benchmark is rerun.

## Fewer bugs reach production (measured)

In the planted safety bug benchmarks, Xo prevented or caught 22 to 27 of
30 bugs, against 17 for Go with strict analyzers and 12 for plain Go. The
ordering held for two model families as authors (Claude and Codex):
[bench/results/codex-2026-10-09.md](../bench/results/codex-2026-10-09.md),
[bench/results/planted-xo-2026-10-07.md](../bench/results/planted-xo-2026-10-07.md).

Where it comes from:

- Value semantics: no accidental sharing of maps, lists, or structs
  between tasks or callers, which removes a class of data races.
- Effects and capabilities: a function declares what it may touch (`uses
  net`, `fs.read`, `clock`), checked at compile time. Per dependency
  effect ceilings in xo.mod (decision 0086) mean an upgraded library
  cannot start using the network silently.
- Structured concurrency: tasks live in scopes; no leaked goroutines;
  cancellation built in.
- Errors cannot be ignored: Results must be handled, `?` propagates, and
  error frames record where an error passed and with which values
  (decisions 0092, 0104).
- Contracts (`requires`, `ensures`, invariants), fuzzed by `xo test`.
- No nil, no zero value surprises, exhaustive `match`.

## Built for AI agents (the original design goal)

- Diagnostics are JSON with stable XO codes, notes, and machine fixes;
  `xo check --fix` applies the unambiguous ones (decision 0096); about 130
  habits from Go, Rust, TypeScript, Python, Swift, and Kotlin get a
  targeted message and fix (decision 0087).
- Measured: Codex went from 3/10 to 8/10 (low effort) and 9/10 (high
  effort, after fix) first try passes in two days, from diagnostics work
  alone ([codex-xo-r3-2026-10-09.md](../bench/results/codex-xo-r3-2026-10-09.md)).
- Semantic queries and edits (`xo query`, `xo edit rename`), an MCP
  server, `xo serve`, and an editor language server (`xo lsp`).

## Debugging production

- Deterministic tests: the same seed gives the same task interleaving, so
  a flaky concurrency failure reproduces exactly.
- Record and replay: record a run (effects, scheduling, Go library calls)
  and replay it offline with the same output and the same fault.
- Crash bundles and live task dumps (`xo dump`).

## Performance and deployment when Go is not enough

- Native backends (preview at 1.0, decision 0089): hello world 86 to
  310 KB against Go's 2.4 MB; `--no-gc` for predictable memory.
- HTTP faster than Go in our benchmarks: HTTP/1.1 141k to 143k vs 121k
  req/s; HTTP/2 on one connection 87.8k vs 58.2k
  ([http2-2026-10-10.md](../bench/results/http2-2026-10-10.md)).
- The Go backend cross compiles and ships static binaries like Go.

## Alongside Go, not instead of it

- `use go` imports Go packages directly (decision 0041), and the default
  backend emits Go: Go's toolchain, libraries, and deployment stay.
- Adopt gradually: one service or package in Xo beside existing Go code.

## Honest costs today

- Pre 1.0: 0.x releases may still change the language (each breaking
  change ships an `xo fix` rewriter, decision 0089).
- Unfamiliar: a new model starts with a lower first try rate until the
  diagnostics teach it.
- Small ecosystem of its own; Go's libraries are the fallback.
- Native backends are preview; some features are still XO1201 there.
- Go backend binaries are larger than plain Go's (hello world 3.6 MB vs
  2.4 MB, decision 0098).
- One project, one maintainer.

## In one line

Go's deployment and ecosystem, with stronger safety guarantees, tooling
built for agents, reproducible debugging, and an optional native path
for speed and size.

## Precedents: languages on top of Go, and self-hosting

From the assistant's knowledge (training data to mid 2026; small
projects may be missing), recorded 2026-10-10:

- Languages compiling to Go exist but are few, and none known reached
  self-hosting: Go+ (XGo, compiler in Go), Borgo (Rust-like, compiler in
  Rust), Oden (functional, compiler in Haskell, abandoned around 2016),
  and smaller experiments.
- Self-hosting precedents: Go itself (C compiler machine translated to Go
  for Go 1.5 in 2015, both kept until the Go one matched; the model for
  decision 0130), Nim and V (compile to C, self-hosted), and TypeScript
  (self-hosted for years, now being ported to Go for speed, TypeScript 7
  "Corsa").
- What would be distinctive for Xo: a self-hosted compiler that builds
  itself through either backend (Go, or a single native binary with no Go
  toolchain), with diagnostics designed for agents and its own effects
  and contracts checked on itself.
