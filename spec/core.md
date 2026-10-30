# Xo Language Specification (Core)

Status: draft 0.10 (2026-10-10; 0.9 released as toolchain v0.9.0, decision 0089)
Edition: preview (the only edition before 1.0, decision 0155)
Companion: `spec/std.md` (prelude methods and standard library surface)

Xo is a small, statically typed, garbage collected language for backends and
web services. It keeps what makes Go productive (few keywords, one obvious way,
fast builds, static binaries, a strong standard library, a compatibility
promise) and is designed so that an AI agent can write, check, build, and debug
programs for any OS with fast, structured, unambiguous feedback.

## Changes in 0.10

- **User capabilities on every backend** (decision 0307): a program's own
  type converted to `Fs`, `Proc`, `Env`, `Net`, `Stdio`, `Clock`, or
  `Rand`, and an `Os` built with a literal, work on the native backends as
  on the Go backend (they were XO1201); `Rand.shuffle` is derived from
  the value's `int`.
- **Raw multi-line strings** (1.6, decision 0320): `r"""..."""` (and
  `r#"""..."""#` to hold `"""`) is a multi-line string with no escapes
  and no interpolation, its common leading indentation removed as for
  `"""`.
- **Embedded files** (3.1, decision 0304): `const X: Str = embed("path")`,
  also `Bytes` and `embed.Dir`, puts files into the program at build time
  on every backend; `XO1506` to `XO1509`.
- **Moves end with their path** (5.3, decision 0300): a move of a
  `Buf`, `StrBuilder`, or `BytesBuilder` on a path that ends (`return`,
  `break`, `continue`, the error side of `?`, a `Never` call) no longer
  reaches the code after it, so `return Some(out.freeze())` inside a loop
  or an `if` is accepted; `XO0510` keeps its meaning.
- **Debug interpolation** (1.6, decision 0296): `${x:?}` writes the Debug
  text of `x`, as `x.debug()` does, so a `Result`, `List`, or `Option`
  needs no `.debug()`. `xo vet` notes `${x.debug()}` with the fix
  (`XO0535`).
- **Discarded pure results** (2.6, decision 0258): an expression statement
  that drops the value of a call that only computes it (`xs.reverse()`,
  `s.trim()`, a function with no effects) is error `XO0209`, fixed by
  `xs = xs.reverse()` or `let _ = f()`.
- **`xo build --stamp NAME=VALUE`** (10.1, decision 0165): sets a `const
  NAME: Str` of the main module for one build (versions, commits);
  `XO1501` when there is no such constant. `std/cli` writes shell
  completion scripts (std.md 14.5).
- **Formats** (std.md 16 to 19, decision 0164): `std/csv` and
  `std/toml` (every backend), `std/regexp` and `std/archive` (gzip, tar,
  zip; Go backend, native builds report XO1201, planned).
- **Files** (std.md 3.1, 3.5, decision 0163): streaming `File` handles,
  `append`, `copy`, permission bits, symbolic links, `glob`, advisory
  locks, and `Env.config_dir`/`cache_dir`. Go backend; native builds
  report XO1201 (planned).
- **Terminals** (std.md 3.6, 15, decision 0162): `Stdio` tells whether
  each stream is a terminal and its width, reads secrets without echo,
  and reads binary input; `std/term` colors output following NO_COLOR
  and FORCE_COLOR. Every backend.
- **Commands and children** (std.md 3.8, decision 0161): `Proc.cmd`
  builds a `Cmd` (directory, environment from an `Env`, input, the
  terminal, merged streams) run by `output`, `run`, or `spawn` (a
  `Child` with streaming reads and writes); `Proc.pipeline` pipes
  commands. Go backend; native builds report XO1201 (planned).
- **`std/cli`** (14, std.md 14, decision 0160): command line parsing
  for tools: flags of five types and repeated ones, positional and rest
  arguments, subcommands, `--`, variables as fallbacks, generated help,
  and usage errors with exit status 2 (`parse_or_exit`). Every backend.
- **Logging is the effect `log`** (6.1, std.md 6, decision 0156): the
  `std/log` functions perform `log`; a `pub` function that logs declares
  `uses log`. `xo fix` adds it to existing code (XO0701 with a fix).
- **`xo audit`** (10.1, decision 0156): per dependency effects, lock
  state, ceiling, escape hatches and vouches; attenuations and the
  capabilities passed into dependencies.
- **Capability attenuation** (6.3, std.md 3.2, 3.5, 3.7, decision 0156):
  `os.net.only(hosts)`, `os.env.only(names)`, and `os.proc.only(cmds)`
  return narrowed capabilities; `NetErr` and `ProcErr` gain `Denied`.
  Native builds have `only` too (decisions 0172 and 0175).
- **Effect lock** (4.3, decision 0156): xo.lock records each
  dependency's effects (`<path> effects <labels|none>`); an update that
  widens them fails every load with `XO1317` until `xo mod
  accept-effects` (or `xo get --accept-effects`) records them.
- **Vouches** (4.2, decision 0156): a required module's `use c` lines and
  `use go` of impure Go packages count only with a `vouch` line in the
  consuming module's xo.mod for that exact version, else `XO1316`; an
  unvouched `use c` binding performs `ffi` whatever its own `uses` says.
- **`edition preview`** (4.2, 13, decision 0155): until 1.0 the only
  edition is `preview`, which `xo init` writes. `edition 2026` stays an
  alias of it during the 0.x releases; `xo check` gives the fixable note
  `XO1315` on it and `xo fix` rewrites it. The compatibility promise of 13
  starts with the first frozen edition, named at 1.0.
- **Receiver order** (5.2, decision 0326): a by value receiver that is a
  variable or a field path is read after the call's arguments, as a `var`
  receiver is (0142); `t.size(t.add(v))` sees the item `add` pushed.
- **Assignment order** (5.2, decision 0140): an assignment computes its
  value before it reads the binding it stores into, so changes the value
  makes to that binding are kept (the native backends dropped them).
- **`for b in bytes`** (3.5, decision 0138): `Bytes` is iterable and
  yields each byte as a `UInt8`.
- **`while let`** (3.5, decision 0137): `while let Some(v) = expr { }`
  loops while a refutable pattern matches, and `while let v = opt` is
  the Option shorthand, as for `if let`.
- **Real-time functions** (6.5, decision 0133): `uses realtime` on a
  function declaration forbids allocation (`XO1401`), loops without a
  bound (`XO1402`), calls of functions that are not real-time, function
  values, and interface methods (`XO1403`), recursion (`XO1404`), and
  blocking (`XO1405`). Native builds defer the frees of a real-time
  section to its end and fault with `XO-F007` when it allocated.
- **Loop bounds** (3.5): `for x in xs bound N { }` and
  `while cond bound N { }`, a contract on any loop: a constant range
  longer than N is `XO1402`; otherwise starting iteration N + 1 faults
  with `XO-F006`. `bound` is a contextual word.
- **Builds skip inline test blocks** (9.2, decision 0389): `xo build`
  and `xo run` neither declare nor check a `test` block, as they read no
  `_test.xo` file (decision 0348); an error in one fails `xo check` and
  `xo test`, not a build.

## Changes in 0.9

Meaning changes: std/http over TLS speaks HTTP/2 when both sides offer
it, on every backend (below), and
one variable passed to two `var` parameters of a call is an error
(`XO0503`, 5.2, decision 0111; the Go backend shared it, the native
backends reported XO1201). A value that holds a handle (`Mutex`,
`Chan`, `Task`, `Conn`, ...) or a function value is not comparable: `==`,
`Map` keys, `Set` elements, and `derive Eq`/`Hash` on it are `XO0404`
on every backend (2.9, decision 0112; the Go backend compared the
handles, the native backends reported XO1201). Native support grew
(`--no-gc`, 10.1):

- Interface types as values (2.4) build natively (decision 0100).
- Newtypes (2.8, decision 0097: construction, `e.0`, `Email(p)`, Debug
  text, equality, Map keys) and list patterns (3.5) build natively.
- Float32, `Secret[T]`, `Buf[T]`, order of `derive Ord` types, and the
  std value methods listed in decision 0101 build natively.
- `std/path`, `task.semaphore`, `task.once`, and `task.map_concurrent`
  build natively (std.md 7, 9).
- Every Fs method and attenuation, `Stdio.write`, and standard input
  reads (7.2, 7.5: canceled by a cancellation or a stop request) build
  natively (decision 0102).
- Error frames (11.1) are recorded natively and printed in main's error
  report (decision 0104).
- `std/time` builds natively (std.md 13, decision 0094), with the same
  per use zone data and output as the Go backend; so do
  `Duration.from_secs`, `scale`, `ceil_seconds`, and
  `Time.parse_rfc3339` (2.6).
- std/http serves and calls HTTPS on every backend (std.md 4.6, decisions
  0105 and 0106): `http.tls_cert`, `Server.with_tls`, `Client.with_roots`,
  and `TlsErr` are new; the native backends speak TLS with Mbed TLS, so an
  `https` URL is no longer `XO1201` there. Over TLS, servers and clients
  speak HTTP/2 when ALPN agrees on `h2` (decision 0121; the Go backend's
  server spoke HTTP/1.1 only before, its client already used HTTP/2);
  plain `http` stays HTTP/1.1.
- `use c "<header>" [link "<lib>"]... [as name] [uses ...]` calls C from
  native builds (4.5, decisions 0093 and 0116): bindings generated from
  the header (`xo bind --c`), scalars, Str, Bytes, flat structs, and
  opaque handles at the boundary, the effect `ffi` (6.1) unless the line
  narrows it, `XO1104` and `XO1105` for headers and names that do not
  bind, and `XO1201` on the Go backend.

## Changes in 0.8

From the spec audit of 2026-10-09 (`bench/results/spec-audit-2026-10-09.md`):
no meaning changes, the spec now says what is not implemented yet.
Method syntax stays receivers (decision 0090), `Map` keeps insertion
order (0091), error frames are lazy and always on (0092), and foreign
calls will be recorded at the boundary (0093); `std/time` is accepted as
specified in std.md 13 (decision 0094).

- `xo build --ci` exists: it fails before building on an unformatted file
  or a `dbg` left in code (10.1, 10.5, 11.4).
- 10.6 lists every diagnostic and fault code with its severity, kept equal
  to the toolchain's registry by a test. `XO1001` is reserved.
- `--json` is accepted by the commands that report results, not by every
  command; `xo test` takes `--cases=N`, not `--fuzz` (10.1).
- **[later]** markers for what is specified but not built: `xo why`,
  `xo debug`, the task line of error
  traces, automatic spans and OpenTelemetry export (std/trace's spans and
  std/metrics exist, 11.6), SBOMs, `std/sys`, `OpenApi` and `Proto`
  derives.
- Modules (4.2, 4.3; decision 0086 steps 1 and 2): `require <path>
  <version>` of Xo modules with minimal version selection, git fetch into
  a read only module cache, `xo.lock` hashes verified by every command,
  `xo get`, `xo mod tidy`, `xo vendor`, `--offline`, and `xo.work`
  workspaces (`--workspace=off`). New codes `XO1301` to `XO1308` (10.6).
  An import from a module that is not required is `XO1306`.
- From the Codex r2 rerun (`bench/results/codex-xo-r2-2026-10-09.md`): a
  closure whose body never completes has result type `Never` when nothing
  else determines it (3.3); `x.display()` is callable on every `Display`
  type, derived and std errors included (std.md 1); several `match` arms
  on one line keep parsing, with a fix that splits them (`XO0175`).
- Effect ceilings and checked semver (4.2; decision 0086 steps 3 and 4):
  `require <path> <version> uses none` (or `uses net, fs.read`) in xo.mod
  and `use <dir> uses ...` in xo.work bound what a dependency's functions
  may declare; a call above the ceiling is `XO1309` at the call site.
  `xo get` prints the API and effect changes of every version change
  (`-u` upgrades every requirement). `xo publish [--check]` compares the
  exports and effects with the previous tag and rejects a breaking minor
  or patch release (`XO1310`, `XO1311`); it never tags or pushes.
  `xo init` writes xo.mod and gitignores xo.work. Module paths ending in
  `/vN` (N >= 2) are major version N of the same repository; the suffix
  is skipped for the local name (4.1).
- `std/time` (std.md 13, decision 0094) is implemented on the Go backend:
  zones with per use embedded data (`--tzdata=system` reads the host
  database), civil dates and times with explicit daylight saving gaps and
  overlaps, and layouts with named fields. A constant layout is checked at
  compile time: `XO0420` for an unknown field (10.6), `XO0177` for date
  patterns of other languages. Native backends report it as `XO1201`
  (planned).
- Import direction rules (4.2, 10.3; decision 0095): `deny <pkg> ->
  <pkg>, ...` lines in xo.mod, with `/...` patterns; a forbidden `use` is
  the new error `XO1312` at that line, a rule naming no package is
  `XO1301`. `xo query imports` and `xo query deps` list the `use` lines.
- `std/time` errors display as sentences (std.md 13.5): `ZoneErr` and
  `CivilErr` no longer derive `Display`.
- Newtypes (2.8; decision 0097): `e.0` reads (and on a `var` assigns) the
  underlying value, and the pattern `Email(p)` matches it, irrefutably
  when `p` is (`let Email(s) = e`). Go backend; native builds report
  `XO1201` as for every newtype (native support added in 0.9).
- `xo check --fix` (10.1, 10.2; decision 0096) applies every unambiguous
  machine fix, reformats the changed files, and reports what remains with
  the list of applied fixes; plain `xo check` stays read only. The MCP
  `check` tool takes `fix`.
- From the second pass of the spec audit: `x.debug()` is callable on
  every value, as std.md 1 says every type implements `Debug` (it was
  `XO0601` on concrete types); reproducible builds are stated for the same
  source directory (12.1), which a test now checks.
- Modules in repository subdirectories (4.2; decision 0086): the module
  `github.com/acme/tools/cli` in `cli/` with tags `cli/vX.Y.Z`; fetch,
  version selection, xo.lock, vendoring, `xo get`, and `xo publish`
  handle the prefix. Nested modules are left out of the module around
  them. `XO1311` now also reports a subdirectory module whose path does
  not end with its directory.
- Go calls are recorded (11.3; decision 0093 stage 1), replacing "Go calls
  are not recorded": calls with boundary values only are answered from
  the trace by `xo replay`; calls with a handle or a callback run again
  and are reported (``re executed foreign call `<name>` ``); pure
  functions are `replay rerun` (std's pure packages, and xo.mod lines
  `replay rerun go <path> [names]`, 4.4). Old traces still replay.
- Release builds (8, decision 0099): `xo build --release` compiles out
  `requires(debug)` and `ensures(debug)` on every backend and fails while
  a workspace overrides a required module (`XO1308`, formerly reported
  only by `--ci`). `--ci` does not imply `--release`; `xo run`, `xo test`,
  and plain `xo build` are debug builds.
- Error frames are lazy (11.1, decision 0092, Go backend): a frame keeps
  copies of the parameter values and formats them, at most 64 bytes'
  worth, only when the trace is printed; output is unchanged. About 10
  times cheaper per propagated error with short parameters, 29 times
  with 90 byte ones, and independent of parameter size. Frames are on in
  release builds; `--release --error-args=false` drops the parameter
  values (`(...)`).
- Module proxies, checksum log, definition hashes (4.2, 4.3, 10.3;
  decisions 0124 to 0126, completing 0086): `XO_PROXY` and `XO_NOPROXY`
  with a read only proxy protocol and `xo mod proxy serve`; `XO_SUMDB`
  and `XO_NOSUMDB` check every first fetch against a signed Merkle log
  (new `XO1313`, `XO1314`; `XO1302` now also covers proxies); `xo query
  hash` and the definitions list of `xo publish`.

## Changes in 0.7

Decisions 0041 (Go packages) and 0042 (two phase shutdown).

- Request scoped arenas are on by default for HTTP handlers (5.5; decision
  0066): an implementation detail with no observable effect, off with
  `--no-arenas`.

- Go packages with `use go` (4.4).
- Two phase shutdown (7.5).
- `Mutex.with_read`, shared read only access (5.4, std.md 2.7; decision
  0063). A closure parameter declared `var` where the expected function
  type takes it by value is error `XO0401` (it changed a copy only).
- `BytesBuilder`, the move only builder of `Bytes` (5.3, std.md 2.7;
  decision 0078).
- Warning `XO0305` is not reported for a `_` arm that hides at least 8
  variants and more than half of the enum (3.5; decision 0080).
- A rune literal where an integer type is expected is its code point
  (1.6); `Int(r)`, `Rune(n)`, and `Rune.try(n)` convert between `Rune` and
  the integer types (2.1; decision 0079).
- `xo build --hybrid` compiled hot leaf loops to native assembly inside Go
  backend builds (10.1; decision 0084); deprecated since 2026-10-22 (owner):
  it is a plain Go backend build with a warning, and will be removed.
- The native backends check `ensures` and struct invariants and unwind
  faults (deferred code, scopes, Mutexes, `task.isolate`, a 500 from an
  HTTP handler) as the Go backend does (3.5, 8, 9.1; decision 0085).
- Habit diagnostics: error `XO0177` (syntax from another language), and
  habit notes and fixes on existing codes, for Go, Rust, TypeScript,
  Python, Swift, and Kotlin forms (10.2; decision 0087). No program that
  was accepted is rejected.
- `clock.sleep_or_cancel(d)` returns cancellation as `ClockErr.Canceled`
  instead of unwinding; reading standard input is a cancellation point,
  and a stop request ends it too (7.2, 7.5; decision 0088).

## Changes in 0.6

Driven by the independent replant of planted safety bugs
(`bench/results/planted-xo-2026-10-07.md`). Decision 0038.

- Reading a `Map` by index, or changing an element in place through it, is
  error `XO0410`; use `get`, `get_or_fault`, or `update`. `m[k] = v` stays (3.6).
- Warning `XO0305`: a `_` arm hides variants of an enum declared in the same
  module (3.5).
- Warning `XO0804`: a value read in one `Mutex.with` is used, or acted on, in a
  later `with` on the same mutex (5.4).
- Regression tests for shrunk contract fuzz failures go in the module's
  `regressions_test.xo`, not `testdata/regressions/` (9.2).
- Go packages with `use go` (4.4), with the `go` effect.
- `for x in ch` over channels (3.5, 7.3); `task.Once` and
  `task.map_concurrent_until` in std.
- Two phase shutdown: the first signal requests a stop (`task.stop_requested`,
  `task.stopping`), cancellation follows after a grace period (7.5).
- Std: `Map.get_or_fault`, hex and base64 on `Bytes`, `Time.from_unix_millis`,
  `Atomic.max`, `Range.to_list`, `Patch.apply_to`/`apply_opt`, and `std/path`.

## Changes in 0.5

Driven by the stage 2 benchmark (`bench/results/stage2-xo-check-2026-10-07.md`).
Decision 0037.

- XO0520: a mutated copy of a collection element must be stored again; reading
  it afterwards no longer counts (5.2).
- `is` may be combined with `&&` and `||` (3.5, 15).
- Each arm of a tail `if` or `match` is in return position (2.6).
- `testing` is usable in helper signatures in `_test.xo` files (9.2).

## Changes in 0.4

Driven by the P1 parser and checker, the planted bug benchmark, and the harder
tasks (`bench/results/planted-and-harder-2026-10-07.md`,
`bench/results/check-corpus-v0.3.md`). Decisions 0028 to 0035.

- Task groups that can be stored in structs, `os.tasks` for program lifetime
  work, `try_send`/`try_recv`, `task.map_concurrent` (7.1, std).
- Discarding a `Result` with `let _ =` is an error; use
  `.ignore_err("reason")` (2.6).
- Effect arguments of generic types may be omitted (6.2).
- `x is Pattern` tests a pattern (3.5). All compound assignments, tuple
  fields `t.0`, local `const`, qualified types in `from`, doc comments on
  fields and variants.
- XO0520 also covers `.or_err(...)?` copies and values mutated after being
  stored in a collection (5.2).
- `Mutex.with` keep and fail pattern documented (5.4).
- Grammar readings from the parser and typing readings from the checker are
  written into the relevant sections (1.3, 1.6, 2.1, 2.6, 2.10, 3.3, 7.1, 15).
- Streaming HTTP bodies, `Fs.write_only`, fuller `Set`, `testing.run_main`
  (std).

## Changes in 0.3

Driven by the second paper test (`bench/results/paper-2026-10-07-v0.2.md`).
Decisions 0021 to 0027.

- String interpolation is `${expr}`; `{` and `}` are literal in plain strings.
  Raw strings `r#"..."#` may contain quotes (1.6).
- `defer` blocks are shielded from cancellation (3.5, 7.2).
- XO0520 applies only to values copied out of a collection (5.2).
- `?` works on `Option` in functions returning an optional (2.6).
- A line starting with `.` continues the previous line (1.3).
- Implicit conversions are listed in one place: there are three (2.10).
- Block and loop typing, generic literal inference, qualified variant
  patterns, module local names, pure means no capability effects, `uses (...)`
  allowed on declarations, process signals (various sections).

## Changes in 0.2

Driven by the paper test in `bench/results/paper-2026-10-07.md`. Decisions
0011 to 0020 in `knowledgebase/decisions/`.

- Explicit failure with `return Err(e)` and tail `Err(e)` (2.6). New `Never`
  type (2.1).
- `Ok Err Some None` replace `ok err some none`. `result` and `old` are
  contextual inside `ensures` only (1.4, 2.5, 2.6).
- Full pattern syntax including `_`, literals, ranges, or patterns, nested
  patterns, positional and named variant fields (3.5).
- Unused binding rule relaxed for parameters and `_` names (3.1).
- Effect variables and effect subsumption (6.2). Multiple effects in a
  function type are parenthesized.
- Scope rules: background tasks, exit behavior, error propagation; `scope ...
  timeout` removed in favor of `task.with_timeout` (7).
- Cancellation is observable as a `Canceled` error from fallible blocking calls
  (7.2).
- `Mutex.with` signature and write back rule (5.4); lost update check `XO0520`
  and `Map.update` (5.2).
- Test blocks get an implicit `os: TestOs`; `std/test` renamed `std/testing` (9.2).
- Logging is ambient and untracked (6.1).
- Slicing with ranges, closure literal grammar, trailing commas, const struct
  literals, `{}` is always an empty map (1.6, 3.x, 15).

## 0. Design goals

1. **No guessing.** Every name resolves to exactly one definition, found by
   reading local code. No overloading, implicit conversions, inheritance,
   macros, `init()`, or hidden control flow.
2. **The compiler is an API.** All tool output is structured JSON with stable
   codes and machine-applicable fixes.
3. **Errors cannot be ignored, nil does not exist, matches are exhaustive.**
4. **Authority is explicit.** Side effects are declared in signatures and
   performed only through capability values passed down from `main`.
5. **Concurrency is structured.** No task outlives its scope. No shared
   mutable state without an explicit synchronized type.
6. **Every target is checked on every build.** No build tags.
7. **Every failure is reproducible.** Effects are recordable and replayable.
8. **Compatibility.** Code that builds under an edition keeps building under
   that edition forever, from the first frozen edition named at 1.0 (13).
   New editions ship with automatic rewriters.

### Staging

Each section is tagged with the phase in which it is implemented.

- **[P1]** Prototype: `xo` frontend written in Go, emits Go source, uses the Go
  toolchain for codegen, GC, scheduler, and cross compilation. Since
  decision 0215 the toolchain is written in Xo (compiler/); the Go
  backend (Go source, the Go toolchain) remains one of its backends.
- **[P2]** Own runtime: deterministic scheduler, full record/replay, task tree.
- **[P3]** Own backend and content addressed code store.

The language semantics do not change between phases. Only implementation
quality and tooling depth change.

**[later]** marks a feature this draft specifies but the toolchain does not
implement yet, in a section otherwise implemented. The spec audit
(`bench/results/spec-audit-2026-10-09.md`) lists every such gap.

### Experimental sections

None. The five rules that were experimental through draft 0.6 (value
semantics, required `uses` on `pub` functions, no module declaration, block
scoped `defer`, enforced naming with `pub`) were accepted on 2026-10-08 from
the benchmark evidence (decisions 0003, 0004, 0006, 0009, 0010).

---

## 1. Lexical structure [P1]

### 1.1 Source files

- Encoding: UTF-8. Extension: `.xo`.
- A **module** is a directory. All `.xo` files in a directory belong to the
  same module and share one namespace. There is no module declaration in the
  file: the directory path is the module identity.
- `use` declarations are per file, as in Go.
- Files ending in `_test.xo` contain only tests and test helpers.

### 1.2 Comments

```
// line comment
/// doc comment, attaches to the next declaration (Markdown)
```

There are no block comments. Fenced `xo` code blocks inside doc comments are
rendered by `xo doc`; compiling and running them as examples under `xo test`
is planned ([P2]).

### 1.3 Statement termination

Newlines terminate statements, using Go's rule: a newline ends a statement if
the last token on the line is an identifier, literal, `)`, `]`, `}`, `?`, or one
of `return break continue`. Semicolons are not written by programmers and are
removed by `xo fmt`.

Exception: a line whose first token is `.` continues the previous line, so
method chains can be split:

```
http.json(429, {"error": "rate limited"})
  .with_header("Retry-After", "${secs}")
```

A line whose first token is a declaration clause (`uses`, `requires`,
`ensures`, `invariant`, `derive`), the `->` of a result type, `else` after a
`requires`, or the `{` that opens a function body, also continues the
declaration above it.

A line cannot start with a binary operator: after `a > 0` at the end of a
line, `|| b > 0` on the next would begin a new statement. End the first
line with the operator instead (`a > 0 ||`, then `b > 0`). A line
starting with `||`, `&&`, `??`, `|`, `^`, `+`, `/`, `%`, `<<`, `>>`, a
comparison, or a spaced `*` or `&` is error `XO0178`, whose fix moves the
operator up (decision 0136). `-`, `!`, `~`, and `..` start expressions,
so a line starting with them is a new statement.

Consequences: `} else {` and `} else if` must be on the same line as the
closing brace. A list that spans lines needs a trailing comma after the last
element. Trailing commas are allowed in every comma separated list.

### 1.4 Keywords (28)

```
as        break     const     continue  defer     else      ensures
enum      fn        for       if        in        interface invariant
let       match     pub       requires  return    scope     select
struct    test      type      use       uses      var       while
```

Predeclared identifiers (cannot be redefined at top level):

- Values and functions: `true false self target fault dbg`
- Variants: `Ok Err` (of `Result`), `Some None` (of `Option`), usable without a
  type prefix everywhere.
- Types: those in section 2.1 and the prelude interfaces in `spec/std.md`.
- In test blocks only: `os`, `expect`, `testing`.

Contextual words (meaningful only in one position, usable as identifiers
elsewhere): `derive` (after a type body), `from` (in an enum variant),
`with` (postfix struct update), `after` (in a `select` arm), `seed` (after a
test name), `result` and `old` (inside `ensures`), `is` (binary operator
position), `bound` (after a loop header's expression, 3.5), `realtime` (in
a declaration's uses clause, 6.5).

To keep parsing unambiguous, an unparenthesized struct literal may not appear
in an `if`, `while`, `for`, `match`, `requires`, or `ensures` expression (the
same rule as Go). Write `if (p == Point{x: 0, y: 0}) { ... }`.

### 1.5 Identifiers and naming

- Identifiers: `[A-Za-z_][A-Za-z0-9_]*`.
- Naming is enforced by the compiler (error `XO0101`), not left to linters:
  - Types, enum variants, interfaces: `UpperCamel`.
  - Functions, methods, variables, fields, modules: `snake_case`.
  - Constants: `UPPER_SNAKE`.
  - Effect variables: `snake_case`.
- Exported names are marked with `pub`, not by capitalization.

### 1.6 Literals

```
42  1_000_000  0xff  0b1010  0o755        // Int
3.14  1e-9                                // Float
true  false                               // Bool
'a'  '\n'  '\u{1F600}'                    // Rune
"hello ${name}, you have ${count + 1} items" // Str with interpolation
"GET /users/{id}"   "{\"a\": 1}"           // braces are literal
"price: \$5"                              // \$ is a literal $ before {
r"raw: no escapes, no interpolation \d+"
r#"raw, may contain "quotes": {"a": 1}"#
r"""
raw multi line: no escapes, no ${interpolation}, indentation removed
"""
"""
multi line string, common leading indentation is removed
"""
b"raw bytes\x00"                          // Bytes, escapes but no interpolation
5s  250ms  10us  2m  1h                   // Duration
[1, 2, 3]   []                            // List
{"a": 1, "b": 2}   {}                     // Map
#{1, 2, 3}   #{}                          // Set
```

- `${expr}` interpolates any expression whose type implements `Display`.
  `Secret[T]` values render as `<redacted>`. `$` not followed by `{` is literal.
- `${expr:?}` interpolates the Debug text of `expr` instead (decision 0296),
  the text `expr.debug()` returns: `"${xs:?}"` is `[1, 2]`, `"${r:?}"`
  is `Err(NotFound)`, `"${name:?}"` is `"ann"` with its quotes. Every type
  implements `Debug` (std.md 1), so any expression may be formatted this
  way; a type with no Debug text is `XO0404` ("`T` does not implement
  `Debug`, so `:?` cannot format it"). `:?` is the only format spec: it
  follows the whole expression, so `${a + b:?}` formats the sum, and any
  other text after the expression (`${x:.2}`) is `XO0160`.
- Raw strings `r"..."` have no escapes and no interpolation. `r#"..."#` may
  also contain `"`, which makes JSON fixtures easy to write.
- A raw multi-line string `r"""..."""` (decision 0320) has no escapes and
  no interpolation either, and spans lines: its common leading indentation
  is removed as for `"""` (a first line holding only the opening quotes
  and a last line holding only the closing ones are dropped), and CR LF
  reads as LF. `r#"""..."""#`, with any number of `#`, is closed only by
  `"""` and as many `#`, so it may contain `"""`.
- An integer literal takes the integer type expected in context (`Int32`,
  `UInt8`, ...), defaulting to `Int`. It never becomes a `Float`; write `2.0`.
  Its value must fit that type: error `XO0421` otherwise (`300` as an
  `Int8`, a negative literal as an unsigned type). A minus written on the
  literal makes a negative literal, so a type's minimum is `-128` or
  `-9223372036854775808`; a parenthesized literal, `-(128)`, must fit alone.
- A rune literal is a `Rune`, except where an integer type is expected (an
  annotation, a parameter, the other operand of an operator, a pattern on
  an integer, a `const` of an integer type): there it is that type's value
  of its code point, so byte level code reads `b[i] == '"'` and
  `match c { 'a'..='z' => ... }` on a `UInt8` (decision 0079). The code
  point must fit the type; `Int8` and `UInt8` take only ASCII (`'é'` is two
  bytes in UTF-8, so it is never one byte): error `XO0401` otherwise.
- Duration literals are an integer followed by `ns`, `us`, `ms`, `s`, `m`,
  or `h`. `1.5s` is not a literal; write `1500ms`.
- Escapes in `Str` and `Bytes`: `\n \t \r \0 \\ \" \' \$ \xHH \u{...}`.
  In `Str`, `\xHH` is at most `\x7f`.
- `{}` in expression position is always an empty `Map`. An empty block used as
  a value is written `()`.

---

## 2. Types [P1]

### 2.1 Builtin types

| Type | Description |
|---|---|
| `Bool` | `true` or `false` |
| `Int` | 64 bit signed. Overflow faults. |
| `Int8 Int16 Int32 Int64` | Sized signed integers. Overflow faults. |
| `UInt8 UInt16 UInt32 UInt64` | Sized unsigned integers. Overflow faults. |
| `Float` | 64 bit IEEE 754. `Float32` also exists. Converting an integer to `Float` or `Float32` rounds once to the nearest value, ties to even, as Go does. |
| `Rune` | Unicode scalar value |
| `Str` | Immutable UTF-8 text |
| `Bytes` | Immutable byte sequence |
| `Duration`, `Time` | Monotonic duration and wall clock instant |
| `List[T]` | Immutable persistent vector |
| `Map[K, V]` | Immutable persistent hash map, insertion ordered |
| `Set[T]` | Immutable persistent hash set, insertion ordered |
| `Buf[T]` | Mutable, move only growable buffer (section 5.3) |
| `Chan[T]` | Channel handle (section 7) |
| `Mutex[T]`, `Atomic[T]` | Shared mutable state (section 5.4) |
| `Secret[T]` | Value that is redacted in every output path |
| `Option[T]`, written `T?` | Optional (section 2.5) |
| `Result[T, E]` | Success or failure value (section 2.6) |
| `(A, B)` | Tuple |
| `fn(A) -> B ! E uses fx` | Function type (section 6.2) |
| `()` | Unit |
| `Never` | The type with no values. Type of `return`, `break`, `continue`, `fault(...)`, `proc.exit(...)`, and infinite loops. Converts to every type. |

Representation note (2026-10-26, not a language rule): with list packing
(decision 0369, Accepted 2026-10-25 and on by default; `--pack-lists=off`
turns it off) a `List[T]` element on the native arm64 and amd64 backends
and on LLVM takes the width of `T` for `Int8`, `UInt8`, `Int16`, `UInt16`, `Int32`,
`UInt32`, `Rune` and `Float32` (1, 2 or 4 bytes), 8 bytes for `Int`,
`UInt64`, `Float` and `Duration`, and 8 bytes a word for a larger `T` (a
`Str` is 16); `List[Bool]` is 1 byte an element and `Bytes` is packed. A
`List[UInt8]` of a million elements is 1 MB, as a `Bytes` of them is. The
Go backend stores Go's own sized types.

A `Str` is always valid UTF-8: literals are, operations on `Str` keep it, and
std makes outside bytes valid where they become a `Str` (std.md 0, decision
0259), so slicing faults only at an offset inside a sequence of valid text.

There are no implicit numeric conversions, except that an index or slice bound
of any integer type is widened to `Int` (3.6). Conversions are calls on the target
type: `Float(i)`, `Int(f)` (truncates toward zero, faults on NaN or out of
range), `Int32(x)` (faults out of range), `Int32.try(x) -> Int32?`. Parsing
text is `Int.parse(s) -> Int?`, `Float.parse(s) -> Float?`.

A `Rune` is not a number, but converts both ways with an integer type
(decision 0079): `Int(r)` (and every sized integer type, faulting out of
range: `UInt8('é')` faults) is its code point; `Rune(n)` from any integer
type is the character with code point `n` and faults when `n` is not a
Unicode scalar value (negative, a surrogate `0xd800` to `0xdfff`, or above
`0x10ffff`); `Rune.try(n) -> Rune?` is `None` there instead. A `Rune`
does not convert to `Float` (`XO0409`, with a note pointing at `Int(r)`).

Float comparison (decision 0053; the same for `Float32`):

- `==` and `!=` are IEEE 754: NaN is not equal to anything, itself
  included, and `-0.0 == 0.0`. Structural `==` on structs, tuples, enums,
  options, lists, and recursive (boxed) values compares Float fields this
  way, so a value holding a NaN is not equal to itself (`x == x` is
  false).
- `<`, `<=`, `>`, `>=` use the total order of `Ord` (std.md 1): every
  NaN sorts after `+Inf` and equals every other NaN in the order, and
  `-0.0` and `0.0` are equal in the order. So `NaN < NaN` is false,
  `NaN <= NaN` is true, `NaN > x` is true for every non NaN `x`,
  `-0.0 < 0.0` is false, and `!(a < b)` is always `a >= b`. Because the
  two families differ, `a <= b && a >= b` does not imply `a == b` (both
  NaN).
- Unary `-` flips the sign of every Float, zero included: `-0.0` is
  negative zero.

`Duration` may be negative (`d.abs()` gives the magnitude).

Operators on `Duration` and `Time`:

| Expression | Type |
|---|---|
| `d1 + d2`, `d1 - d2` | `Duration` |
| `d * n`, `n * d`, `d / n` (n: `Int`) | `Duration` |
| `d1 / d2` | `Int` (truncated) |
| `t + d`, `t - d` | `Time` |
| `t1 - t2` | `Duration` |
| comparisons | `Bool` (both are `Ord`) |

Methods (`d.seconds() -> Float`, `d.scale(f: Float) -> Duration`, and more) are
listed in `spec/std.md`.

### 2.2 Structs

```
pub struct User {
  pub id: Int
  pub name: Str
  pub email: Str?
  created: Time
}
```

- Construction names every field: `User{id: 1, name: "a", email: None, created: now}`.
  Type arguments of a generic struct are inferred from the fields or the
  expected type: `Job{id: 1, input: "x"}` is a `Job[Str]`.
  Fields with a declared default may be omitted:
  `struct Config { port: Int = 8080 }`, so `Config{}` is valid.
- Update syntax creates a modified copy: `u with {name: "b"}`.
- Structs are values. Assignment copies (cheaply: fields are shared because all
  builtin collections are immutable).
- Fields may be separated by newlines or commas.
- A field may name its key on the wire between its name and its type
  (decision 0277): `kind as "type": Str`, `pub api_version as
  "apiVersion": Str`, `created as "Created": Int = 0`. In Xo code the
  field is still `kind`; the derives that map fields to outside names use
  the wire name: `Json` keys (and TOML keys, which std/toml decodes
  through JSON) and `SqlRow` columns (2.9). The name is a plain string
  literal, not empty (`XO0412`).
- The wire string may carry Go's tag options after a comma (decision
  0309). The one option is `omitempty`: `email as "email,omitempty":
  Str?`, or `as ",omitempty"` to keep the field's default key (its name,
  or what `case:` makes of it). `derive Json`'s encoder leaves such a
  field out when its value is empty as Go defines it: `None`, `false`, a
  zero number, `""`, or an empty `List`, `Map`, `Set`, or `Bytes`. A
  struct or an enum value is never empty. Decoding is unchanged: a
  missing key takes the field's default. Another option is `XO0422`.
  Unlike Go, `as "-"` is the key `-`, not a skipped field.
- A field may override the shared wire name for one format (decision
  0315), after its name or its `as` string: `json "..."`, `sql "..."`,
  `xml "..."` (`json`, `sql`, and `xml` are contextual words, not
  keywords).
  - The override replaces the name and the options for that format only.
  - Each format's name is its own string, else the `as` string, else the
    field's name under its derive's `case:` rule.
  - Examples: `created_at as "createdAt" sql "created": Int?` has the JSON
    key `createdAt` and the column `created`; `note json "memo,omitempty"
    sql "note": Str?` names each format alone.
  - A `json` string takes `omitempty`. A `sql` string takes no options
    (`XO0422` for one). An `xml` string is Go's tag text, for `derive Xml`
    (decision 0315; below, 2.9).
  - A format given twice on a field is `XO0423`. A string for a format the
    struct does not derive is `XO0424`.
- `derive Json(case: camel)` (also `snake`, `pascal`, `kebab`, and the
  same argument on `derive SqlRow`) converts every field without its own
  wire name: `api_version` is `apiVersion` under camel, `ApiVersion`
  under pascal, `api-version` under kebab; a field's `as "key"` always
  wins. An unknown argument or case is `XO0413`; two fields with one key
  under a derive the struct has are `XO0411`; a wire name on a struct
  with neither derive is the warning `XO0414`.

### 2.3 Enums (sum types)

```
pub enum LoadErr {
  NotFound(id: Int)
  Db(msg: Str)
  Timeout
}
```

- Variants may carry named fields. Construction uses names:
  `LoadErr.NotFound(id: 42)`. A single field variant may also be constructed
  positionally: `LoadErr.NotFound(42)`.
- The enum prefix may be omitted when the expected type is known (in a
  `match` arm, an argument, a return, a typed binding, or nested inside one of
  those): `return Err(NotFound(id: 42))`.
- Enums from other modules are qualified with the module: `task.Timed.TimedOut`,
  `http.ClientErr.Canceled`. Type arguments of generic enums are inferred:
  `task.Timed.Failed(e)`.
- Enums are closed. `match` must be exhaustive (error `XO0301`). Variants with
  a field of type `Never` cannot be constructed and may be omitted from a
  `match`.

### 2.4 Interfaces

```
pub interface Store {
  fn get(self, id: Int) -> User ! LoadErr uses net
  fn put(self, u: User) -> () ! LoadErr uses net
}
```

- Satisfied structurally and implicitly, as in Go.
- Interfaces may be used as types (dynamic dispatch) or as generic constraints.
- `Never` satisfies every interface.
- A type satisfies an interface through its methods whether or not they are
  `pub`. Calling a non `pub` method by name from another module is still an
  error.
- Every derivable name (`Display`, `Ord`, `Json`, ...) is also an interface and
  may be used as a constraint: `fn reply[T: Json](v: T)`.

### 2.5 Optionals

- `T?` is `Option[T]`: either `Some(v)` or `None`. There is no nil, null, or
  zero value pointer.
- A plain `T` is accepted where `T?` is expected and is wrapped in `Some`
  automatically (one of the three implicit conversions, section 2.10).
- Unwrapping:
  ```
  if let Some(e) = user.email { send(e) } else { log.warn("no email") }
  let e = user.email ?? "unknown@example.com"
  let e = user.email.or_err(LoadErr.NotFound(id: user.id))?
  let e = user.email ?? return None
  let n = fields.get(k) ?? continue
  ```
  - The right side of `??` may be `return v`, `break`, or `continue`
    (decision 0321): an expression of type `Never` that leaves when the
    left side is `None`, so `let x = opt ?? return r` is the four line
    `match opt { Some(v) => v  None => return r }`. A `return` is checked
    against the function's result as a statement `return` is, and
    `break` and `continue` against an enclosing loop (`XO0607`). It takes
    only an Option on the left (`XO0406`); the Result form is below
    (2.6). `xo vet` suggests it for the four line `match` (`XO0906`), on
    an Option and on a Result.
  `if let e = opt` (without `Some`) is also accepted as shorthand for
  `if let Some(e) = opt`.
- In a function that returns `U?`, `opt?` yields the value or returns `None`:
  ```
  fn retry_after(resp: http.Response) -> Duration? {
    let secs = Int.parse(resp.header("Retry-After")?.trim())?
    1s * secs
  }
  ```
  Using `?` on an `Option` in a function that returns a `Result` is error
  `XO0204`, with a fix that inserts `.or_err(...)`. A test body is the
  exception (9.2): there `opt?` fails the test.

### 2.6 Results, errors, and explicit failure

- A function that can fail declares its error type after `!`:
  `fn load(id: Int) -> User ! LoadErr`. Such a function returns
  `Result[User, LoadErr]` to its caller.
- `!` with no type means `! Error`, the builtin open error interface.
  `fn main(os: Os) !` is `fn main(os: Os) -> () ! Error`.
- `T ! Never` is the same type as `T`: a function that cannot fail.
- **Returning.** In a function declared `-> T ! E`, a returned value (a tail
  expression or `return x`) may be:
  - a `T`, which means success, or
  - `Err(e)` with `e: E`, which means failure, or
  - any `Result[T, E]` value, returned as is.

  ```
  fn parse_port(s: Str) -> Int ! ConfigErr {
    let n = Int.parse(s).or_err(ConfigErr.NotANumber(s))?
    if n < 1 || n > 65535 {
      return Err(ConfigErr.OutOfRange(n))
    }
    n
  }
  ```
  If `T` is itself a `Result`, success must be written `Ok(x)` explicitly
  (error `XO0203` otherwise).
- `r ?? e => body` unwraps a Result and names its error (decision 0321):
  ```
  let v = fetch(id) ?? e => return Err(LoadErr.Fetch(e))
  let n = parse(s) ?? _ => continue
  let w = parse(s) ?? e => fallback(e)
  ```
  It is `match r { Ok(v) => v  Err(e) => body }`: the value is the
  success value, and `body` (an expression, a block, or `return`,
  `break`, `continue`) runs with `e` bound to the error and either gives
  a value of the success type or leaves. `_` discards the error; an
  unused `e` is `XO0110` as in any match arm. The left side must be a
  Result (`XO0406`), and `r ?? default` with no arm on a Result stays
  `XO0406`, so an error is never dropped silently. `xo vet` suggests it
  for `match r { Ok(v) => v  Err(e) => return w }` and the `break` and
  `continue` forms (`XO0906`).
- Each arm of a tail `if` or `match` is in return position, so arms may mix
  `T` values and `Err(e)`: `match x { 0 => 1  _ => Err("no") }`.
- **Propagating.** `expr?` on a `Result[T, E2]` yields the `T` or returns the
  error from the enclosing function. `E2` must convert to the enclosing error
  type `E`:
  - `E2` is `E`, or
  - `E` is `Error` and `E2` implements `Display`, or
  - `E` has a `from E2` variant.
- **from variants** wrap other error types:
  ```
  enum ApiErr {
    BadRequest(msg: Str)
    Load(from LoadErr)   // `?` on a `! LoadErr` call wraps into ApiErr.Load
  }
  ```
  A `from` variant has one positional field: construct with
  `ApiErr.Load(LoadErr.Timeout)`, match with `Load(inner)`.
- **Mapping.** `r.map_err(f)` converts the error inline:
  `json.decode[User](b).map_err(fn(e) { ApiErr.BadRequest(msg: "${e}") })?`.
- **Handling is mandatory.** A `Result` value that is never used (not
  propagated with `?`, matched, passed, or returned) is error `XO0201`. A
  `Result` bound to a name that is never used is also `XO0201`, whatever the
  name (including `_x`).
- **Discarding needs a reason.** `let _ = expr` on a `Result` is error
  `XO0206`. Write `expr.ignore_err("why this failure does not matter")`,
  which yields `T?` and records the reason, so every discarded error is
  searchable and reviewable:
  ```
  let _ = dst.remove(tmp).ignore_err("best effort temp file cleanup")
  let nick = nickname_field(fields).ignore_err("nickname is optional") ?? ""
  ```
  `r.ok()` remains for when failure simply means "no value".
- A closure whose tail is a `Result` is a failing closure; the `Result` is
  returned as is. A closure declared `-> T` with no `!` cannot fail.
- A non `Result` value may be discarded as an expression statement, except
  the value of a call that only computes it: one that performs no effect
  (section 6), mutates no `var` receiver or argument, and reaches no
  handle, interface, or function value through its receiver or arguments.
  `xs.reverse()` or `s.trim()` as a statement is error `XO0209` (values
  are not changed in place); its fix assigns the result back
  (`xs = xs.reverse()`). Write `let _ = f()` to discard such a result on
  purpose. A `defer` statement's call is not reported (it runs for what
  it does at exit, or to fault there).

### 2.7 Generics

```
pub fn map[T, U](xs: List[T], f: fn(T) -> U) -> List[U] { ... }
pub fn largest[T: Ord](a: T, b: T) -> T { if a > b { a } else { b } }
```

- Square brackets, Go style. Constraints are interfaces. Effect variables are
  declared in the same list (section 6.2).
- Explicit type arguments at a call: `json.decode[User](body)`.
- Monomorphized at compile time. Type arguments are inferred at call sites when
  unambiguous; otherwise error `XO0402` lists the candidates.
- No higher kinded types, no specialization, no variadic generics.

### 2.8 Type aliases and newtypes

```
type UserId = Int                   // alias, interchangeable with Int
type Email struct(Str)              // newtype, distinct, construct with Email("x")
type Handler = fn(http.Request) -> http.Response ! Error uses net
```

A newtype has the equality, hashing, and JSON of its underlying value
(decision 0097): `Email("a@x.io")` encodes as `"a@x.io"`, and a newtype
field of a `derive Json` struct needs no derive of its own (a newtype
whose underlying type is Json is Json):

- `e.0` is the underlying value, typed with the newtype's type arguments
  (`Wrap[Int]`'s `.0` is `Int`); on a `var` it is assignable like a tuple
  field. Any other index is `XO0601`.
- The pattern `Email(p)` (`mod.Email(p)` for another module's newtype)
  matches when `p` matches `e.0`; it is irrefutable when `p` is, so
  `let Email(s) = e` destructures. `Email(..)` matches any value. Without
  parentheses, with other than one positional pattern, or against another
  type it is `XO0302`. When the value is an enum with a variant of the
  same name, the variant is meant.
- Construction and reading are open wherever the type is visible; a type
  that must guard its value is a struct with a non `pub` field.
- `derive Ord` after the declaration (`type Name struct(Str) derive Ord`)
  orders the newtype by its value: `<`, sorting, `min`, `max`, and
  `T: Ord` code see `Name("a") < Name("b")` as `"a" < "b"`. The value's
  type must be `Ord` (else `XO0404`). A newtype is not ordered without
  it, and `Ord` is the only derive it takes (`XO0404` for any other: its
  equality, hashing, and JSON are its value's already; decision 0390).

```
type Email struct(Str)

fn domain(e: Email) -> Str {
  let Email(s) = e
  s.split("@").last() ?? ""
}

fn is_blank(e: Email) -> Bool { e.0.trim() == "" }
```

### 2.9 Derived behavior

Every type automatically implements:

- `Debug`: structural printing used by the debugger, error frames, and `dbg()`.
- `Eq` and `Hash` when all fields do.

A type is *comparable* when it implements `Eq`: every field, enum payload,
tuple element, and element or type argument is comparable. Function values
and handles are not (decision 0112): `Chan`, `Mutex`, `Atomic`, `Scope`,
`Task`, `Listener`, `Conn`, `ServerConn`, `File`, `FileLock`, `Child`, `task.Group`, `task.Semaphore`, `task.Once`,
`http.Router`, `http.Server`, `http.Client`, `http.BodyWriter`,
`http.TlsCert`, `sql.Db`, `sql.Tx`, and the std/testing fakes
(`FakeNet`, `FakeClock`, `MemFs`, `FakeEnv`, `FakeStdio`, `FakeProc`).
A value that holds one, directly or in a field, payload, tuple, or
collection, cannot be compared with `==` or `!=`, cannot be a `Map` key
or `Set` element, and its type cannot `derive Eq`, `Hash`, or `Ord`
(`XO0404`, with a note naming the field that holds the handle). Compare
an identifying field instead (`a.id == b.id`). Interface values compare
through their dynamic value and stay comparable; one whose dynamic value
holds a handle or a function is equal to nothing, itself included (`==`
is false, as for a function value), on every backend; the opaque Go values of
`use go` bindings keep Go's equality (4.4). Automatic `Debug` is
unaffected.

Opt in with `derive` on the declaration:

```
pub struct User { ... } derive Display, Ord, Json, OpenApi
```

- Derived `Display` prints the same text as `Debug`:
  `NotFound(id: 42)`, `User{id: 1, name: "a"}`. Implement `Display` by hand
  (`fn (e: ApiErr) display() -> Str`) for user facing text.
  Std's error enums implement `Display` by hand and print plain messages
  (`PATH: no such file or directory`, `connection refused`; decision 0358);
  their `Debug` keeps the variant form.
- `Json`, `OpenApi`, and `Proto` derives generate codecs and schemas from the
  same declaration so wire formats and docs cannot drift. JSON rules are in
  `spec/std.md`. **[after 1.0]** `OpenApi` and `Proto` are planned for
  after 1.0 (owner, 2026-10-09; additive, so not a 1.0 blocker): the
  derives are reserved, accepted only when every field derives them too
  (no builtin type implements them yet), and nothing is generated.
- `SqlRow` (structs only) generates a decoder from a std/sql row: each field
  is read from the column of the same name, or of its wire name or
  `case:` rule (2.2; `spec/std.md` 10.4).

- `Xml` (structs only, no type parameters) gives the struct Go's
  encoding/xml Marshal and Unmarshal through `xml.encode_value(v,
  indent)` and `xml.decode[T](text)` (`spec/std.md` 34, decision 0315).
  - A field's tag is its `xml` string, else its `as` string, else its
    name, in Go's tag text: `name`, `name,attr`, `,chardata`, `a>b>c`,
    `,omitempty`, and `-`. `innerxml` and `comment` are not yet
    supported (`XO0426`).
  - Field types are Str, Int, Float, Bool, a struct that derives Xml,
    each alone, as an Option, or in a List (`XO0425` for another type, a
    struct with type parameters, or a struct that does not derive Xml).
  - The expansion is ordinary Xo: the checker parses generated functions
    (`xml_write__T`, `xml_zero__T`, `xml_merge__T`, `xml_from_tree__T`,
    `xml_encode__T`, `xml_decode__T`) into the module as a file of its
    own, so every backend compiles the same code. A diagnostic in them is
    reported at the field. They are not declarations of the program:
    `xo symbols`, `def`, `refs`, `doc`, and the formatter leave them out.
  - A call of `xml.encode_value` or `xml.decode` is rewritten to the
    generated function of its type; a type parameter or a struct that
    does not derive Xml is `XO0427` or `XO0404`.

### 2.10 Implicit conversions

There are exactly four, and each applies only where the target type is
already known:

1. A `T` where a `T?` is expected becomes `Some(T)`.
2. A value where an interface type is expected (a parameter, a field, a
   collection element, a return) becomes that interface, if the type
   implements it. This includes passing a capability fake where the
   capability interface is expected.
3. An error at a `?` site converts to the enclosing error type (2.6).
4. An element of a list literal written as the argument for a
   `List[go.Any]` parameter of a `use go` function (Go's `...any`)
   becomes a `go.Any`, as `go.any(x)` would (4.4, decision 0412); a type
   with no Go value is `XO1106`.

An `if` or `match` whose type is already known (an annotated `let`, an
argument, a field, a return) passes that type to each arm, so each arm
converts on its own: `let s: Stream = if tls { tls_conn } else { conn }`
and `let x: Int? = if c { 1 } else { None }` check. Without a known
type, conversions are not applied when joining branches: the arms must
already agree (`let y = if c { 1 } else { None }` is `XO0401`, branches
have different types). Conversions are not applied to the `else` value
of `requires`. Comparing `opt == v`
wraps `v` in `Some`.

Everything else, including all numeric conversions, is explicit.

---

## 3. Declarations and statements [P1]

### 3.1 Bindings

```
let x = 10            // immutable binding
var total = 0         // mutable binding
total += x
let (q, r) = (7 / 2, 7 % 2)   // a pattern binds each name
var (lo, hi) = (0, 10)        // every name mutable (decision 0384)
const MAX_USERS = 1000
const DEFAULT = Config{port: 8080}
```

- `const` may appear at top level or in a block. A block constant's
  initializer must be a compile time value: literals, other constants,
  struct and enum construction, and collection literals of those
  (`XO0408`).
- A top-level constant's initializer may also call functions (decision
  0332): `const RFC3339 = time.layout("...")`, `const CRC = make_table(0xedb88320)`.
  Such a constant is computed once per process, at its first read, and
  shared by every task: a reader that comes while another task computes
  it waits. The initializer must perform no effect (`XO0415`), and the
  value may hold no handle (`Mutex`, `Chan`, `Atomic`, `Task`, a file;
  `XO0416`), so a constant is an immutable value, never a global
  variable. A fault in the initializer is stored, and every read of the
  constant, the first and each later one, raises it again. A read of a
  constant during its own initialization (through a function it calls)
  faults with `XO-F009`.
- A top-level constant may embed files at build time (decision 0304):
  `const PAGE: Str = embed("static/index.html")` (the file's text, valid
  UTF-8), `const LOGO: Bytes = embed("logo.png")` (its bytes), or
  `const SITE: embed.Dir = embed("static")` (std/embed: every file a
  path or glob names, `*`, `?`, `[a-z]`, `**`; a directory brings its
  files except names starting with `.` or `_`, unless the pattern starts
  with `all:`). The path is one string literal, relative to the directory
  of the declaring file and inside the module (no absolute path, `..`, or
  symbolic link), and the constant declares its type. The constant is a
  literal of the files' contents from the check on, so every backend
  embeds the same bytes, and a change to an embedded file rebuilds what
  reads it. An embed may hold 64 MiB and a program's embeds 256 MiB. A
  module that declares its own `embed` uses it. Errors: `XO1506` (no file,
  too many, too large), `XO1507` (outside the module), `XO1508` (a `Str`
  that is not UTF-8), `XO1509` (any other use). An `embed.Dir`'s keys
  are those relative paths, so `embed("static")` holds
  `static/index.html`; `read`, `read_text`, or `contains` on such a
  constant (or a literal `.sub` of one) with a literal path it does not
  hold is the warning `XO1511`, with the key that ends with the path as a
  fix. A path computed at run time is not checked.
- Unused local bindings, local constants, and imports are errors (`XO0110`). Not covered:
  function and closure parameters, names starting with `_`, and `_` itself.
  `xo fix` renames an unused binding or local constant to `_name` (removing
  it could drop its value's effects) and removes an unused import. `xo check --wip` reports `XO0110` and
  `XO0209` (section 2.6) as warnings, for checking code still being written;
  a normal check and every build reject them (decision 0293).
- Shadowing in the same block is an error. Shadowing in a nested block is
  allowed.

### 3.2 Functions

```
/// Returns the user with the given id.
pub fn fetch_user(db: Db, id: Int) -> User ! LoadErr
  uses net
  requires id > 0
{
  let row = db.query_one("select id, name, email from users where id = $1", id)?
  User{id: row.int("id")?, name: row.str("name")?, email: row.str_opt("email")?, created: row.time("created")?}
}
```

- The last expression of a block is its value. `return` exits early. An empty
  body `{}` has type `()`.
- A block whose last item is a statement (an assignment, `let`, `for`,
  `while cond`, `defer`) has type `()`.
- `while { ... }` with no `break` has type `Never`, so it may end a function
  body of any type (all exits are `return`).
- Parameters are immutable unless declared `var` (in out, see 3.3). Named
  arguments are allowed and recommended when a call has more than two
  parameters of the same type: `transfer(from: a, to: b, amt: 5)`. Positional
  arguments must come before named ones.
- Default parameter values are allowed: `fn listen(addr: Str, backlog: Int = 128)`.
- No variadic functions. Pass a `List` or `Map`.

### 3.3 Closures

```
let double = fn(x: Int) -> Int { x * 2 }
xs.map(fn(x) { x * 2 })                         // types inferred from context
srv.handle("GET /hi", fn(req) { http.text(200, "hi") })
let load = fn(id: Int) -> User ! LoadErr uses net { store.get(id)? }
```

- Parameter and return types may be omitted when the expected function type
  is known.
- Closures capture by value. Mutating a captured `var` inside a closure is
  error `XO0502`. To share mutable state with a closure, capture a `Mutex` or
  `Atomic` handle.
- Creating a closure performs no effects; calling it does.
- `return` inside a closure returns from the closure.
- Effects of a closure are inferred from its body unless declared.
- A closure whose body never completes normally (it ends in `while { ... }`
  without `break`, `fault(...)`, or another `Never` expression) and whose
  result type nothing else determines has result type `Never`, so
  `s.spawn_background(fn() { while { ... } })` needs no type argument
  (`Task[Never, E]`).

### 3.4 Methods

```
fn (u: User) display_name() -> Str { u.name }

pub fn (var a: Account) deposit(amt: Money) requires amt > 0 {
  a.balance += amt
}
```

- A `var` receiver mutates the caller's binding in place (in out semantics). It
  can only be called on a `var` binding (error `XO0501`), a `var` parameter, or
  a field of one (`st.users.put(k, v)` where `st` is `var`).
- Methods may be declared only in the module that declares the type.

### 3.5 Control flow and patterns

```
if cond { ... } else if other { ... } else { ... }   // also an expression

for x in xs { ... }
for (i, x) in xs.indexed() { ... }
for (k, v) in m { ... }
for job in jobs_chan { ... }  // receives until the channel is closed and drained
for b in data { ... }         // each byte of a Bytes, as a UInt8
for i in 0..10 { ... }        // half open range, 0..=10 is inclusive
for _ in 0..3 { ... }
while cond { ... }
while let Some(job) = queue.pop() { ... }  // until the pattern fails
while { ... }                 // infinite loop
for x in xs bound 64 { ... }  // a loop bound: a contract (below, 6.5)
while cond bound 1000 { ... }

match code {
  200..=299 => "ok"
  429 | 503 => "retry later"
  _ => "error"
}
```

- `if` without `else` has type `()`.
- `for` iterates over a List, Buf, Set, Map (as `(k, v)` tuples), Chan, Range,
  or Bytes. Over `Bytes` it yields each byte as a `UInt8`, in order
  (decision 0138); a byte compares with an ASCII rune literal
  (`b >= 'a'`) and converts with `Int(b)`. Index pairs come from
  `xs.indexed()` on a List; for Bytes, loop over `0..data.len()` and
  read `data[i]`. A `Str` is not iterable: choose `.runes()` or
  `.bytes()`.
- `while let pattern = value { ... }` computes `value` before each
  iteration and runs the body while it matches the pattern, with the
  pattern's names bound in the body; the loop ends at the first value
  that does not match (decision 0137). Any refutable pattern works
  (`while let Tok.Num(n) = next()`), and `while let v = opt` is
  shorthand for `while let Some(v) = opt`, as with `if let` (2.5).
  `bound N` follows the value: `while let Some(x) = q.pop() bound 64`.
- `x is Pattern` is a `Bool` expression that tests a pattern without binding
  names: `expect(r is Err(Timeout))`, `if resp is Ok(_) && retries < 3`.
  `is` is a contextual word in operator position (precedence of `==`), so it
  combines with `&&` and `||` like a comparison.
- Compound assignment works for `+= -= *= /= %=` on `var` bindings and their
  fields. There are no bitwise compound assignments.
- A value counts as stored again (for XO0520) by any later `push`, `put`,
  `insert`, `set`, `add`, `update`, or `m[k] = v` of it, including later in
  the same loop.
- Tuple fields are `t.0`, `t.1`, ...
- `match` is an expression. Arms are separated by newlines. An arm body may be
  an expression, a block, or a single statement such as an assignment (type
  `()`): `Timeout => last = Some(AttemptErr.Timeout)`.
- Labeled loops: `outer: for ... { break outer }`.
- **Loop bounds** (decision 0133). `bound N` after the iterated value of a
  `for` or the condition of a `while` states that the loop runs at most N
  iterations; N is a constant Int expression (literals, `const` names,
  `+ - * / %`), not negative, else `XO1402`. A `for` over a range with
  constant ends that is longer than N is `XO1402`. Otherwise the bound is
  checked at run time on every backend: starting iteration N + 1 faults
  with `XO-F006` (`loop ran past its bound of N iterations`). Like a
  `requires` clause it is a contract (9.1): `xo test` fuzzes a function
  with a bounded loop. Real-time functions (6.5) need a bound on every
  loop whose count is not constant. `while { }` takes no bound; write
  `while true bound N { }`.
- `defer stmt` or `defer { ... }` runs when the enclosing **block** exits (not
  the function, unlike Go), in reverse order, including on errors, faults, and
  cancellation. Values are read when the deferred code runs.
- Deferred code is **shielded from cancellation**: inside it, cancellation
  points do not report cancellation, so cleanup such as removing a temp file
  works after a timeout. A shielded `defer` that runs longer than 5 seconds
  of real time faults (`XO-F005`); the Go backend checks this when the
  deferred code returns, and not in test worlds, whose clock is virtual.
- The native backends (`--backend=llvm`, `--backend=arm64`) run deferred
  code on every way out alike, faults included (decisions 0070, 0085).
- Deferred code that faults ends there; the deferred code registered
  before it and that of the blocks around it still runs, and the new
  fault replaces the one being unwound, if any (the last fault is
  reported). The task is no longer shielded afterwards. Every backend
  behaves this way (decision 0085, amended 2026-10-10).

**Patterns** are used in `match`, `let`, `var`, `if let`, `while let`, and `for`:

| Pattern | Matches |
|---|---|
| `_` | anything, binds nothing |
| `x` | anything, binds `x` |
| `42`, `"GET"`, `'a'`, `true` | equal literal |
| `1..=9`, `'a'..='z'` | value in inclusive range |
| `p1 \| p2` | either (both must bind the same names) |
| `(a, b)` | tuple |
| `Some(p)`, `None`, `Ok(p)`, `Err(p)` | prelude variants |
| `Timeout`, `http.ClientErr.Canceled` | unit variant, bare or qualified |
| `NotFound(p)` | variant fields positionally, in declared order |
| `NotFound(id: p)` | variant fields by name; `..` ignores the rest: `Db(msg: m, ..)` |
| `Variant(..)` | variant with any fields |
| `User{id: p, ..}` | struct fields by name |
| `[]`, `[first, ..]`, `[.., last]`, `[a, .., z]` | list shape (`..` at most once, anywhere) |
| `(p)`, `()` | grouping, unit |

Patterns nest: `Some(Ok(v))`, `Err(LoadErr.NotFound(id))`.

An unguarded `_` (or catch all binding) arm that hides variants of an enum
declared in the same module is warning `XO0305`: list the variants, so that
adding one later is a compile error at every `match`. Enums from other modules
may use `_`, since their owners can add variants. A `_` arm that hides at
least 8 variants and more than half of the enum is a default ("these few are
special, the rest are not", as on a token kind enum) and is not reported; the
warning names at most 8 hidden variants, then how many more (decision 0080).
An `UpperCamel` name in a pattern always refers to a variant; bindings are
`snake_case` (1.5), so the two never collide.
`let` requires an irrefutable pattern (`let (a, b) = pair`), and so does
`var` with a pattern (`var (a, b) = pair`, `var [first, ..] = xs`,
`var Point{x, y} = p`; decision 0384): every name it binds is mutable,
as one `var` per name would be.

### 3.6 Indexing and slicing

- `xs[i]` on `List`, `Bytes`: faults when out of range. `xs.get(i)` returns `T?`.
- The index (in a read, an assignment, and `get`) and the bounds of a slice
  may be any integer type, `Int`, `Int8`..`Int64` or `UInt8`..`UInt64`
  (decision 0331): it is widened to `Int` and then bounds-checked. A
  `UInt64` above `Int`'s range wraps to a negative `Int` and so is out of
  range (the fault prints the wrapped value). Any other type, `Rune` and
  `Float` included, is `XO0401`; this is the only implicit numeric
  conversion (2.1 has none other).
- `Map` cannot be read by index: `m[k]` in an expression, or an in place change
  through it (`m[k].f = v`, `m[k] += 1`, `m[k].push(x)`), is error `XO0410`,
  because a missing key is the Map form of a nil dereference. Use `m.get(k)`
  (`V?`), `m.get_or_fault(k, "why the key must exist")`, or
  `m.update(k, init, fn(var v) { ... })`. On a `var` map, the whole value
  assignment `m[k] = v` is allowed and means `m.put(k, v)`.
- `Str` cannot be indexed by position (use `runes()` or slicing).
- Ranges slice: `xs[1..]`, `xs[..n]`, `xs[a..b]` on `List`, `Bytes`, and `Str`
  (byte offsets; slicing a `Str` inside a UTF-8 sequence faults). A range of a
  `Str` or `Bytes` that is not an ordered range of its length faults with
  `slice LO..HI out of range for Str of length N` (`Bytes` in place of `Str`),
  and a `Str` slice whose ends are not UTF-8 boundaries with `slice LO..HI of
  Str splits a UTF-8 sequence`; every backend prints the same text (the corpus
  of internal/stdtest/testdata/faults). Slices of
  immutable values are O(1) and share storage: on native targets the result
  of slicing a `Str` or `Bytes` is a view of the same buffer (no copy), so a
  small view keeps its whole buffer alive (as a Go substring does; there is
  no automatic compaction of a stored view; `s.clone()` makes an explicit
  copy when a piece must outlive a large source; std parsers copy the tokens they keep of a large input, and `xo vet --cost` notes a stored view of a parameter's text, XO0911). A view is never changed in place:
  appending to one, or building on one, copies. A `Str` or `Bytes` value
  holds at most 4294967295 bytes (4 GiB
  less one); an operation that would make a longer one (concatenation,
  appending to a builder) faults with "Str or Bytes longer than
  4 GiB", and `repeat` with "strings: Repeat output over 4 GiB". WebAssembly
  and the Go backend copy a slice (no views) but keep the same limit and
  results. A C ABI export of the native runtime (`xo_x_` names, decision
  0207) may not return a `Str`, because the second word of a view carries
  its offset: the compiler refuses it ("a C ABI export with a `Str`
  result"). Decision 0330.

### 3.7 Holes

`???` is a typed hole, valid anywhere an expression is expected.

- `xo check` succeeds with warnings and reports, for each hole, the expected
  type, the effects in scope, and in scope values or functions that fit
  (diagnostic `XO0900`, see section 10).
- `xo build` rejects programs with holes. `xo run --holes` and `xo test --holes`
  allow them and fault if a hole is evaluated (`evaluated a hole \`???\``),
  on the native backends and the Go backend alike (natively since
  2026-10-22, owner; `--holes` chose the Go backend before).

Holes let an agent write a type correct skeleton first and fill it in steps.

---

## 4. Modules and dependencies [P1, content addressing P3]

### 4.1 Imports

```
use std/http
use std/json as j
use github.com/acme/billing/invoice
```

- The last path segment is the local name unless `as` is given, with `-`
  replaced by `_` (`use acme/rate-limit` binds `rate_limit`). A `/vN` major
  version suffix of a module path is skipped (`use github.com/acme/tick/v2`
  binds `tick`). If the result is
  still not a valid identifier, or is a keyword, `as` is required (`XO0602`).
- A path is segments of ASCII letters, digits, `_`, `-`, and `.` separated
  by `/` (`PathSeg`, section 15). Only the last segment becomes the local
  name, so dashes elsewhere need nothing (`use github.com/xo-lang/xo/lexer`
  binds `lexer`). A leading or trailing `/`, an empty segment, or a
  segment that is exactly `.` or `..` is `XO0158`: import paths are
  absolute, never relative (`use ./lib`), and name each package one way,
  so `deny` rules (4.2) see every import. A `.` inside a segment is fine
  (`example.com`, `.github`, `v1.2`).
- Imports are explicit. There is no dot import or glob import.
- Cyclic module imports are an error.
- `std/internal` holds primitives for std's implementation and the Xo
  toolchain's own modules (decision 0214) and is not part of std (std.md
  does not list it): only std modules and modules whose path is
  `github.com/xo-lang/xo` or below it may import it. For any other module
  `use std/internal` is `XO0601` (module not found), as the native
  backends' intrinsics (`native/...`) are.
- Std-internal names (decision 0319): a `pub` item of a native std
  implementation module (std/native/M.xo, imported as `native/M`) is
  visible to the other native std modules and to nothing else, since
  only std modules may import `native/...` and `use std/M` sees only
  std/M.xo's declarations. A std module calls another's internals this
  way (std/websocket's connection hand-off is
  `native/http`'s `handoff_response`) instead of a registry or a
  marker value; std.md lists none of these names.

### 4.2 xo.mod

```
module github.com/acme/api
edition preview

require github.com/acme/billing v1.4.0
```

`edition` names the module's edition (13); without the line a module is
on the current one. Before 1.0 that is `preview`, the only edition;
`edition 2026`, its old name, is an alias that builds and gets the fixable
note `XO1315` (decision 0155). Any other name stops the check.

**[P1]** Dependencies (decision 0086):

- `require <module path> <version>` requires another Xo module. The module
  path is its git location (`github.com/acme/billing` is fetched from
  https://github.com/acme/billing.git; the module is the repository root
  and its xo.mod declares the same path); the version is a tag `vX.Y.Z`
  (`-prerelease` allowed). A module is required once. `require go
  <module> <version>` pins a Go module for `use go` instead (4.4).
- A module may live in a subdirectory of its repository, as in Go:
  `github.com/acme/tools/cli` is the directory `cli/` of the repository
  `github.com/acme/tools`, its versions are the tags `cli/vX.Y.Z`, and a
  version is that directory's tree at the tag. On github.com, gitlab.com,
  and bitbucket.org the repository is the first three path elements; on
  other hosts the longest prefix of the path that git answers for is the
  repository. A directory holding its own xo.mod is never part of the
  module around it (it is not fetched, hashed, or vendored with it).
- There is no `replace`, `exclude`, or `retract` (`XO1301`); local copies
  are reached only through a workspace (below).
- Minimal version selection: the build uses, for each module, the highest
  version that any reached xo.mod requires (the main module's, then those
  of every required version, transitively). No ranges, no solver; a
  version changes only when an xo.mod changes.
- Imports resolve inside the main module, the embedded `std`, and the
  selected module versions (the longest module path that prefixes the
  import path). A module must require every module it imports from:
  importing a package of a module xo.mod does not require, even one in the
  build because another module requires it, is `XO1306`.
- Fetching runs git and extracts the tag's tree (`git archive`) into a read
  only module cache; no code from a module runs at fetch or build setup
  (no scripts, no hooks). A version that cannot be fetched is `XO1302`.
- Module proxies (decision 0126): `XO_PROXY` lists where versions come
  from, in order: proxy URLs (`http://`, `https://`), `direct` (git), and
  `off` (no fetching); the default is `direct`. After a comma the next
  source is tried only when a proxy answers 404 or 410, after a pipe on
  any error. `XO_NOPROXY=<patterns>` (comma separated globs over leading
  path elements, `github.com/acme/*`) always goes direct. The protocol is
  read only HTTP: `<path>/@v/list`, `<path>/@v/<version>.mod`, `.zip`
  (the tree, files under `<path>@<version>/`), and `.lock` (the xo.lock
  lines), the path and version escaped as in the module cache (`!` before
  a lower cased capital). A proxy is not trusted: its tree must declare
  the module path and match xo.lock as a git fetch must. `xo mod proxy
  serve [--addr=host:port] [--fetch] [<cache dir>]` serves a module cache
  this way (`--fetch` fills it from git). A malformed `XO_PROXY` is
  `XO1314`.
- Offline (`--offline` on any command, or `XO_OFFLINE=1`) nothing is
  fetched: every version must be in the module cache or vendored, else
  `XO1303`.
- `xo vendor` copies every selected version into `vendor/<module path>/`
  with `vendor/modules.txt`. When `vendor/modules.txt` exists, builds use
  only vendor/ (no cache, no network); a requirement it lacks is `XO1307`.
- Commands: `xo get <path>[@version|@latest|@none]` sets a requirement
  (latest tag by default) and rewrites xo.mod and xo.lock; `xo mod tidy`
  requires every imported module, removes requirements nothing imports
  (unless they raise a version the graph selects), and rewrites xo.lock.

**[P1]** Workspaces: an `xo.work` file at or above the main module makes
the modules it lists build from their directories:

```
xo preview
use ./api
use ./billing
```

- `xo <edition>` names an edition as xo.mod's `edition` line does:
  `preview`, or its old name `2026`, which builds and gets the fixable
  note `XO1315` (`xo fix` writes `xo preview`); any other name is
  `XO1301` (decision 0155).
- A listed module overrides every `require` of it; minimal version
  selection runs over the requirements of all listed modules.
- The workspace applies when it lists the main module; `--workspace=off`
  (or `XO_WORK=off`) ignores it, `XO_WORK=<file>` names another one.
- `xo check`, `test`, `query`, `edit`, and `mcp` take every listed module
  as one program (a rename reaches all of them, atomically).
- xo.lock is never written with workspace overrides (`xo get`, `xo mod
  tidy`, and `xo vendor` ignore the workspace); builds record the local
  directories in build information only. A release build (`xo build
  --release`, 8) and `xo build --ci` fail while a workspace overrides a
  required module (`XO1308`).
- `xo.work` is gitignored by `xo init` in a single module repository;
  a monorepo may commit it.

**[P1]** Effect ceilings (decision 0086 step 3): a `uses` clause after
the version bounds the effects of a dependency.

```
module github.com/acme/api

require github.com/acme/billing v1.4.0 uses none
require github.com/acme/geo v2.1.0 uses net.dial, fs.read
```

- Every call from the requiring module into the dependency (any of its
  packages), and every use of one of its functions as a value, may
  perform only effects within the ceiling (`fs` admits `fs.read`; `uses
  none` admits nothing), else `XO1309` at the call site naming the
  dependency, the ceiling, and the effect. The effects counted are those
  the dependency's function declares; effect variables (6.2) carry the
  caller's own effects and do not count.
- An upgrade whose new version adds an effect to a function this module
  calls therefore fails at that call. A require without `uses` has no
  ceiling. Unknown labels are `XO1301`.
- In a workspace, `use ./billing uses none` in xo.work sets the ceiling
  of that module for calls from the other workspace modules; their
  xo.mod ceilings apply as well.
- `xo get` prints, for every requirement whose version changes, the
  exports added, removed, or changed and the effects added or removed,
  and the exports above the ceiling; `xo get -u` upgrades every
  requirement to its latest version.

**[P1]** Vouches (decision 0156): an escape hatch is code whose
authority the checker cannot see, a `use c` line (4.5) or a `use go` of a
Go package that is not pure (4.4; the pure ones are the std packages
marked `replay rerun` by default there). In a module the build gets as a
required version (the module cache or vendor/), each escape hatch needs a
`vouch` line in the xo.mod of a consuming module: the main module, or a
module of the active workspace.

```
vouch github.com/acme/zip v1.2.0 c "zlib.h" uses none
vouch github.com/acme/img v0.3.1 c "png.h"
vouch github.com/acme/pg v1.0.0 go "github.com/jackc/pgx/v5"
```

- `vouch <module path> <version> c "<header>" [uses <effects|none>]`,
  `vouch <module path> <version> go "<Go import path>"`. The header or
  package is the string of the dependency's `use` line; the version is
  the exact version the build selects (a vouch reviews one version's
  code, so an upgrade needs a new one). Malformed lines are `XO1301`.
- A dependency's own vouch lines are ignored: it cannot vouch for itself
  or for its dependencies. The main module's and the workspace modules'
  own `use c` and `use go` lines need no vouch.
- A vouched `use c` binding performs the vouch's `uses` (the consumer's
  word, not the dependency's), or `ffi` without one. An unvouched escape
  hatch, or one vouched for another version, is error `XO1316` on the
  dependency's `use` line, naming the line to add; an unvouched `use c`
  binding performs `ffi` whatever its own `uses` says. There is no
  machine fix: a vouch is a human review. `xo get` and `xo mod
  accept-effects` refuse a selected version with an unvouched escape
  hatch (`XO1316`, in any of its source files, from syntax) before
  writing xo.mod or xo.lock, `--accept-effects` included: it accepts
  effects, not escape hatches. `xo mod tidy` does not refuse; the
  check then reports them.

**[P1]** Releases (decision 0086 step 4): a version is a `vX.Y.Z` tag of
the repository whose root holds the module, or a `<dir>/vX.Y.Z` tag for a
module in the directory `<dir>`. `xo publish [--check]
[--version vX.Y.Z]` compares the exports (the declarations `xo query api`
lists, in every package of the module) and their effects with the
previous tag:

- A removed export, a changed signature, or an effect added to a
  function needs a major version; in a minor or patch release each is
  `XO1310` (before v1.0.0, a minor release may break and a patch may
  not). Added exports need a minor version.
- Without `--version` the release is the next minor version (the next
  patch before v1.0.0), so a breaking change fails unless a major version
  is asked for. The JSON result lists every change, the breaking ones,
  the bump they need, and the smallest version they allow.
- `XO1311` when the release cannot be made: the module is not in a git
  repository, a module in a subdirectory has a path that does not end
  with that directory, the tag exists or is not above the last release, or
  the major version does not match the module path. Without `--check`,
  uncommitted changes are `XO1311` and the module must load and check
  without a workspace.
- Publishing is tagging: `xo publish` prints `git tag vX.Y.Z && git push
  origin vX.Y.Z` (`git tag cli/vX.Y.Z ...` in `cli/`) and runs neither.
- Major versions 2 and above live at a module path ending in `/vN`
  (`github.com/acme/billing/v2`), in the same repository: the xo.mod of
  those tags declares that path, and a require of a path without the
  suffix takes only v0 and v1 tags (`XO1301` otherwise).

**[P1]** Import direction rules (decision 0095): `deny` lines state which
packages of the module may not import which.

```
module example.com/notes

deny example.com/notes/domain -> example.com/notes/store, example.com/notes/api
deny example.com/notes/store -> example.com/notes/api/...
```

- `deny <pattern> -> <pattern>, ...`: a package matching the left pattern
  may not `use` a package matching any pattern on the right. A pattern is
  a package import path (of this module, a required module, or std), or
  such a path ending in `/...`, which matches the path and every package
  under it (`example.com/notes/...` is the whole module, so "`domain`
  imports only std" is `deny example.com/notes/domain ->
  example.com/notes/...`).
- Every direct `use` line is checked, whether the import is used or not:
  a forbidden one is error `XO1312` at the `use` line, naming the rule and
  its xo.mod line; it has no machine fix (removing an import is a design
  change). Transitive edges are not checked: a chain is caught at its
  first forbidden step.
- Rules apply to the packages of the module whose xo.mod holds them
  (the main module, and each workspace module checked as one);
  a dependency's rules are checked when it is built as a main module.
- A malformed `deny` line, or a pattern that names no package, is
  `XO1301` on its line. `xo mod tidy` and `xo get` keep `deny` lines.
- `xo test` checks first, so a broken rule fails the tests too.
  `xo query imports <package>` and `xo query deps <package>` list the
  `use` lines (10.3).

**[P1]** `gc` lines tune the garbage collector of a program whose main
module this is (decision 0065); they are ignored in imported modules:

```
gc min_heap 64MiB      // floor of the heap goal
gc percent 200         // GOGC: goal growth over the live heap, or `off`
gc memory_limit 1GiB   // soft memory limit (GOMEMLIMIT)
```

Sizes take `B`, `KiB`, `MiB`, `GiB`, `TiB` (or `KB`, `MB`, `GB`, `TB`, powers
of 1000). Without `gc` lines a program's heap goal is the live heap plus 100
percent of it (as Go's GOGC=100), but never below a floor of 2 MiB per
processor (`GOMAXPROCS`), at least 16 MiB; there is no memory limit. The
environment always wins: `GOGC` set turns the floor off and sets the
percent, `GOMEMLIMIT` replaces `memory_limit`, `XO_GC_MIN_HEAP` replaces
`min_heap` (0 turns the floor off).

### 4.3 xo.lock

Written only by the toolchain. Maps every imported definition actually used to
its content hash.

- **[P1]** Hash is per module version (source tree hash), like Go's `go.sum`:
  `<path> <version> h1:<hash>` for the tree of every selected version and
  `<path> <version>/xo.mod h1:<hash>` for the xo.mod of every version
  version selection read. `xo get` and `xo mod tidy` write it; every
  command that loads the program recomputes the hashes from the module
  cache or vendor/ and compares them. A mismatch (a moved tag, an edited
  cache or vendor copy) is `XO1304` and nothing builds; a version with no
  entry is `XO1305` (run `xo mod tidy`). Malformed lines are `XO1301`.
- Pinned Go modules (decision 0356): for each `require go <module>
  <version>` line, xo.lock records Go's own go.sum hashes, `go <module>
  <version> h1:<hash>` (the module zip) and `go <module>
  <version>/go.mod h1:<hash>`, taken from the binding workspace's
  go.sum. Every check compares them with the binding's go.sum: a
  different hash is `XO1304`, a missing line `XO1305`; `xo mod tidy`
  and `xo get` write them, and a rewrite keeps a line no binding has
  fetched yet.
- **[P1]** Effect lock (decision 0156): xo.lock also records, for each
  selected module (not workspace modules), the effects its exported API
  can reach: `<module path> effects <labels|none>`
  (`github.com/acme/billing effects net.dial,stdio`), the union of the
  effect labels in the `uses` clauses of its `pub` functions and methods
  (every package, test files left out). The line is keyed by module
  path, so it survives an upgrade. Every command that loads the program
  compares each module's effects with its line: an effect the line does
  not admit (`fs` admits `fs.read`) is `XO1317` on that line. `xo get`
  and `xo mod tidy` record a module without a line as it is, narrow a
  line whose module needs less, and fail with `XO1317` (writing nothing)
  when a module needs more. `xo mod accept-effects [<module path>...]`
  (all modules when none is named) and `xo get --accept-effects` record
  the wider effects; `xo check` and `xo build` never do. A module with
  no line (a lock written before decision 0156) is not checked.
- **[P1]** Checksum log (decision 0124): with `XO_SUMDB=<name>+<key
  id>+<public key> [<URL>]` (default `off`), the first fetch of every
  module version, from git or a proxy, is checked before it enters the
  module cache: the log's record of the version (its two xo.lock lines)
  must equal the fetched tree's hashes, the record must be in the log's
  signed Merkle tree (inclusion proof), and that tree must extend every
  tree of the log this module cache saw before (consistency proof). A
  failure is `XO1313` and nothing is cached; a log that cannot be
  consulted, or malformed settings, is `XO1314`. `XO_NOSUMDB=<patterns>`
  (as `XO_NOPROXY`, 4.2) names private modules that are never looked up.
- **[P1]** Definition hashes (decision 0125, step 5 of decision 0086):
  every top level definition (function, method, type, constant) has a
  content hash `d1:<base64 SHA-256>` over its syntax tree with
  positions, comments, and doc comments dropped, its own name left out
  (functions and constants; a type's name is part of it), local names
  erased (numbered by first binding: `let`, `var`, patterns, loops,
  closure parameters, type parameters, receivers, labels; the
  parameters of declared functions keep their names, which named
  arguments use), imports by path rather than by local name, and the
  definitions of the same module it references replaced by their hashes
  (mutually recursive ones hashed together). `hash` covers that closure,
  `own` the definition alone. `xo query hash <name>` prints them (10.3);
  `xo publish` lists every definition, exported or not, whose hash
  differs from the previous release (`changed`, or `changed_via` when
  only a definition it uses changed), next to the API comparison. The
  hashes are syntactic (no types): a method called on a value and other
  modules' definitions are referenced by name. xo.lock does not record
  them; decision 0125 proposes how it would.
- **[P3]** Hash is per definition: the normalized AST with local names erased
  and referenced definitions replaced by their hashes. Consequences:
  - Two versions of a dependency coexist without conflict. Types from different
    versions are the same type if and only if their hashes are equal.
  - An upgrade that does not change a definition is a no-op for code using it.
  - A name that does not resolve to a hash fails at once with the nearest real
    names suggested (`XO0601`). APIs cannot be hallucinated silently.

### 4.4 Go packages [P1]

Until Xo has its own ecosystem, Go packages can be used directly (decision
0041):

```
use go "regexp"
use go "github.com/google/uuid" as uuid

fn main(os: Os) ! {
  let re = regexp.must_compile("[a-z]+")
  os.stdio.println("${re.find_string("123abc")} ${uuid.new_string()}")
}
```

- `xo check`, `build`, `run`, and `test` generate bindings automatically into
  `.xo/gobind/` (or `$XO_GOBIND_DIR`); `xo bind <path>` does it explicitly.
  A package outside Go's standard library needs its Go module pinned,
  `require go <module> <version>` in xo.mod (4.2), or the use is
  `XO1319` (decision 0356): its fix adds the line with the version Go's
  module resolution picked, and `xo mod tidy` adds missing pins and
  records their go.sum hashes in xo.lock from the binding (a pin whose
  hashes no binding has fetched and xo.lock lacks is `XO1320`: run
  `xo check` once first). The pin is of the package's own module: a
  package in a nested Go module (aws-sdk-go-v2's `service/s3`, a module
  inside `github.com/aws/aws-sdk-go-v2`'s path) needs its own line, and
  a pin of the enclosing module pins only that module, which the nested
  one resolves beside. Go's
  standard library packages need no line; the Go toolchain's version
  decides them.
- Names are converted to Xo naming: functions and methods to `snake_case`
  (`Split` is `split`, `EncodeToString` is `encode_to_string`), types stay
  `UpperCamel`.
- Types at the boundary: `(T, error)` becomes `T ! go.Error`; a pointer result
  that may be nil becomes `T?`; mutable Go objects become opaque handles;
  a handle keeps Go's equality (2.9): a comparable type the package hands
  out as a value, `T` in a signature or field (`netip.Addr`, `uuid.UUID`),
  compares and hashes by value, a struct it passes only as `*T`
  (`url.Parse`'s `*url.URL`) by pointer, as Go's `==` on the pointers,
  and an interface by its dynamic value; a named Go type is one Xo type
  in every binding that uses it, declared by its own package's binding
  (`net`'s functions take `netip.AddrPort`), which is bound with it
  even when the program does not name that package (decision 0392); a
  type of a package a program cannot import (`internal`) is skipped;
  a parameter of an unexported Go type is skipped, and so is a `[]byte`
  parameter Go writes into (a `Read*` function's, or one named `dst`,
  unless a result is a `[]byte`: a Bytes is a value and cannot carry
  Go's writes back);
  slices and maps are copied in and out; Go's `any` (`interface{}`, or a
  named empty interface) is `go.Any`, an opaque Go value (decision 0412,
  std.md 12): `go.any(x)` makes one from a value with a Go form, the
  elements of a list literal for `...any` convert implicitly (2.10),
  `as_int()`, `as_str()`, ... read one back (None on another Go type),
  and a Go function that fills a value through `any` (`rows.scan`,
  `json.unmarshal`) takes `go.out[T]()`'s `.any()`, read with `.get()`
  after the call; a generic Go function is an Xo generic (decision 0415):
  `any` is no constraint, `comparable` is `Eq`, `cmp.Ordered` is `Ord`, a
  `~[]E` or `~map[K]V` type parameter is `List[E]` or `Map[K, V]`, other
  constraints are skipped, and Go's `slices` and `maps` are not bound
  (the prelude's List and Map methods cover them); a type argument for
  `comparable` or `cmp.Ordered` needs a Go value Go can compare or order,
  else `XO1106`, while one for `any` may be any Xo type, which Go holds
  without looking inside; a generic Go type is a generic handle
  (`lru.Cache[K, V]` is `Cache[K: Eq, V]`, its methods bound with `K`
  and `V`) when its constraints map, and an instance over type
  parameters or types Go and Xo share (`Str`, `Bool`, a type parameter)
  is that handle's instance; a constraint interface (one with a type
  term, such as `interface{ ~int | ~float64 }`) is not a type and is not
  bound; a `context.Context` parameter is
  supplied from the current task (deadline and cancellation); a panic becomes
  a fault.
- A named Go type over a basic type (`time.Month`, `fs.FileMode`) is that
  basic type in Xo, and each method it has with a value receiver is a
  package function named `<type>_<method>` in snake case that takes the
  value first (decision 0416): `time.month_string(m: Int) -> Str`,
  `fs.file_mode_is_dir(m: UInt32) -> Bool`. A pointer receiver method is
  not bound (Xo passes a copy), and neither is one whose name a package
  function already has.
- An Xo function passed to Go as a callback that Go calls during the Go call,
  on the calling task's goroutine, runs as the calling task. A call from
  another goroutine (`sync.WaitGroup.go_`, a timer) runs as a task of its own,
  a child of the calling task (canceled with it), with its own held mutexes
  (decision 0067); under the deterministic scheduler such a call waits for
  a turn like a spawned task and is recorded by `--record` (core 7.4, 11.3).
- A callback whose Go type ends in an `error` result (`func(int) error`,
  `func(string) (int, error)`) is an Xo function `-> T ! go.Error` (`-> ()
  ! go.Error` with no other result; decision 0416): its `Err(e)` is Go's
  error, the Go error `e` came from when Go made it (so `errors.Is` sees a
  sentinel such as `io.EOF`), else one with `e`'s text; `go.error(msg)`
  makes one. A callback with several results (`func() (int, string)`,
  `func(string) (int, bool, error)`) returns them as a tuple, `-> (Int,
  Str)`, its error still the `! go.Error`.
- A variadic of an unnamed Go function type, Go's functional options
  (`optFns ...func(*s3.Options)`), is a List of Xo functions, each a
  callback by the rules above, which Go calls in order (decision 0422):
  `s3.new_from_config(cfg, [fn(o: s3.Options) { o.set_use_path_style(true) }])`,
  or `[]` for none. A variadic of a named function type (`...EOption`)
  stays a List of handles, which Go's option helpers return.
- A struct field or package variable of a Go function type has a setter
  that takes an Xo function, a callback by the rules above
  (`s.set_on_error(fn(msg: Str) { log.send(msg) })`, `hook.set_logf(f)`);
  Go may call it later, from any goroutine.
- A Go function value Go gives Xo (a result, a field or variable read) is
  an Xo function `uses go` that calls it, its arguments and results
  converted as a bound function's are and a final `error` its `! go.Error`
  (`fnval.adder(2)` is `fn(Int) -> Int uses go`; a value of a named
  function type, such as a functional option or `http.HandlerFunc`,
  stays a handle that goes back to Go; in a result it is
  parenthesized, `-> (fn(Int) -> Int uses go) uses go`). Calling a nil Go
  function faults; several results are a tuple. It cannot be passed back
  to Go, and one taking a function is not bound.
- Go's `error` anywhere but a function's final result (a parameter, a
  field, a result before the last) is `go.Error?`, `nil` being `None`
  (`errors.is(err: go.Error?, target: go.Error?) -> Bool`); an `error`
  that is a function's only result stays `! go.Error` (`nil` is `Ok`);
  a Go error variable is a function returning it as a `go.Error`
  (`io.eof()`). A go.Error that came from Go goes back as the same Go
  error (decision 0416).
- A Go pointer to a basic type (`*bool`, `*string`, `*int`, `*float64`)
  is `T?` (decision 0419): `nil` is `None`, otherwise the value it points
  to. It is a copy, not an alias: Xo reads the value when it crosses the
  boundary and passes `Some(v)` to Go as a pointer to a fresh copy of
  `v`, so a write Go makes through the pointer later is not seen (read
  the field again). A parameter Go writes through (sync/atomic, a `*Var`
  function such as `flag.int_var`, an output parameter) is not bound.
- A Go channel Go gives Xo (`<-chan T` or `chan T` as a result, a field or
  a variable) is `go.Recv[T]` (decision 0421): `recv()` waits for the next
  value (None once the channel is closed and empty), `try_recv()` does not
  wait, `each(f)` receives until it is closed. Xo does not send on, close
  or pass back a Go channel (a send only channel or a channel parameter is
  not bound), and a nil one is closed. A waiting `recv` is a cancellation
  point: the task's cancellation or a `task.with_timeout` deadline ends it.
  Under the deterministic scheduler it gives the task's turn away while it
  waits; `--record` keeps each received value that holds no handle. Go
  timer channels (`time.after`, `time.tick`, `Timer.C`) are not bound: Xo's
  clock is a capability they would bypass.
- An Xo program implements a Go interface with functions (decision 0416):
  a binding declares, for each named interface whose methods all map as
  callbacks, a constructor named after the interface (`implement_` and
  the name when that is taken) with one function parameter per method,
  named after it, returning the interface's handle:
  ```
  let w = io.writer(write: fn(p: Bytes) -> Int ! go.Error {
    sink.send(p)
    p.len()
  })
  let h = http.handler(serve_http: fn(w, r) { w.write_header(204) })
  ```
  The functions are callbacks (the rules above: the calling task, or a
  task of its own for a call from another goroutine, with spawn's rules
  for what they capture). A method that fills a byte buffer (`Read(p
  []byte) (int, error)`, `ReadAt`) is a function given the buffer's
  length that returns the bytes (`fn(max: Int) -> Bytes ! go.Error`):
  they are copied into the buffer, more than it holds is a fault, and
  none with no error is `io.EOF`.
- Xo does not convert one handle to another. A handle whose Go type Go
  would assign to another interface of its package, or to one of io's
  I/O interfaces (`Reader`, `Writer`, `Closer`, `ReadCloser`,
  `WriteCloser`, `ReadWriter`, `ReaderAt`, `WriterTo`, `ReaderFrom`,
  `StringWriter`, `ByteReader`) or `fmt.Stringer` when its package depends
  on io or fmt, or to another package's interface that its package's
  signatures name (chi's `Mux.Handle` takes net/http's `Handler`) or that
  a package it imports exports (aws credentials' providers as
  `aws.CredentialsProvider`), has a method that gives it as that
  interface:
  `io.read_all(resp.body().as_reader())`, `buf.as_writer()`,
  `httptest.new_server(mux.as_handler())` (decision 0416 and its
  addendum).
- Every call into Go performs the effect `go`, which `pub` functions must
  declare. Go code can reach files and the network without a capability, so
  `uses go` marks that authority escape in signatures.
  In a required module, a `use go` of a package that is not pure (the
  pure std packages listed below) needs the consuming module's `vouch`
  line (4.2, decision 0156), else `XO1316`.
- Exported names whose signatures cannot be mapped are skipped and listed at
  the top of the generated binding; using one is `XO1102`. A package that
  cannot be bound is `XO1101`.
- C headers are the native backends' counterpart (4.5).
- Calls into Go take part in record and replay (11.3, decision 0093): each
  binding's doc comment says how (`/// Replay: ...`). A pure function can
  be marked `replay rerun` in xo.mod, so a replay calls it again instead of
  answering it from the trace and the trace does not record it:

  ```
  replay rerun go github.com/acme/slug
  replay rerun go os getpid File.name
  ```

  The line names a Go import path and optionally the functions (`name`)
  and methods (`Type.method`) it marks; without names it marks every
  function of the package. The pure std packages (`strconv`, `strings`,
  `bytes`, `unicode`, `unicode/utf8`, `unicode/utf16`, `math`,
  `math/bits`, `regexp`, `regexp/syntax`, `path`, `html`, `net/url`,
  `encoding/hex`, `encoding/base64`, `encoding/base32`) are marked
  already, except functions that take or return a handle of another
  package (`regexp.Regexp.match_reader`).

### 4.5 C headers [P2, native builds]

Native builds (`--no-gc`, 10.1) call C through bindings generated from a
header (decisions 0093 and 0116):

```
use c "tiny.h" link "tiny.c"
use c "zlib.h" link "z" uses none

fn main(os: Os) {
  os.stdio.println("${tiny.tiny_add(2, 40)} ${zlib.crc32(0, b"hello")}")
}
```

- `use c "<header>"` names a header relative to the module directory, or
  one on the C compiler's include path (`zlib.h`). `link "<x>"`, repeated
  as needed, names what to link: a C source, object, or archive relative
  to the module directory (`tiny.c`, `libtiny.a`), or a library name
  without `/` or extension (`z` links `-lz`). `as` sets the local name;
  the default is the header's base name in snake_case.
- The checker binds the header with clang's AST dump
  (`xo bind --c <header>` does it explicitly) into `.xo/cbind/` (or `$XO_CBIND_DIR`): an
  Xo declaration file and a C adapter that the native build compiles and
  links with the program. Function names become snake_case
  (`zlibVersion` is `zlib_version`); struct tags become UpperCamel types.
- Types at the boundary (decision 0093), all copied, none pointing into Xo
  memory after the call returns: `int` is `Int32`, `unsigned` `UInt32`,
  `long` and `long long` `Int`, their unsigned forms `UInt64`, `short`,
  `char`, and their unsigned forms the sized integers, `float` `Float32`,
  `double` `Float`, `_Bool` `Bool`, an enum `Int32`; a `const char *`
  parameter is a `Str` (copied and NUL terminated) and a `const char *`
  result a `Str` (copied; NULL is ""); a const byte pointer followed by an
  integer length parameter is one `Str` (`const char *`) or `Bytes`
  parameter; a struct of such scalars passed or returned by value is an Xo
  struct with the same fields, copied; a pointer to a struct the header
  declares but does not define is an opaque handle type (a result is
  `T?`, None for NULL). Handles are not comparable (2.9) and Xo never frees
  them: call the library's free function. Anything else (output
  pointers, mutable buffers, `char *` results, pointers to defined
  structs, function pointers, arrays, variadic functions) is skipped and
  listed with the reason at the top of the binding; using a skipped name
  is `XO1105`. A header that cannot be bound is `XO1104`.
- Every call performs the effect `ffi` (6.1), which `pub` functions
  declare. A `use c` line may replace it with the effects a human vouches
  for, `uses none` or `uses fs.read, clock`, as an effect ceiling of
  decision 0086 does for a dependency. That claim counts only in the
  module's own code: in a required module the consuming module's `vouch`
  line decides the binding's effects (4.2, decision 0156).
- `use c` is impossible on the Go backend: `xo check`, `build`, `run`, and
  `test` without `--no-gc` report `XO1201` on the line. (`use go` is
  likewise `XO1201` on the native backends.)
- A C crash is not recoverable: it ends the process (8).

---

## 5. Values, mutation, and memory [P1]

### 5.1 Value semantics

All builtin types and user structs, enums, and tuples have value semantics.
There are no pointers and no reference types other than the explicit handle
types (`Chan`, `Mutex`, `Atomic`, capability values, `Buf` by move).

Because values never alias mutably, data races are impossible except through
`Mutex` and `Atomic`, which are synchronized.

**What a copy costs** (informative; decision 0294). The semantics above
never depend on it, but the time does:

- Copying a value (a `let` of another binding, an argument a function
  keeps, a field, list, or map store, a return, a closure's or a task's
  capture) copies its inline part and shares each of its heap fields
  (`Str`, `Bytes`, `List`, `Map`, `Set`, a box of a recursive type, a
  handle) by counting it up. The cost is the number of heap fields,
  counted through nested structs, never the size of their contents. A
  parameter that is only read is borrowed and costs nothing.
- An argument passed to a function value (a closure, a `fn` parameter, a
  named function used as a value) is borrowed too, unless its type is
  the function's result type or one of a tuple result's (an accumulator
  threaded through, `fold`'s `fn(A, T) -> A`, or a `fn(var T)` closure's
  value): that one is handed over, so the body can update it in place.
  The rule depends on the function type alone (decision 0337); a body
  that keeps a borrowed argument (returns, stores, or captures it)
  counts it up then. A move only value of a borrowed position stays the
  caller's for the call; its first write in the body copies it.
- Writing a collection or string that another value still shares copies
  it first, O(n), and the writer then holds its own: later writes to it
  are in place (a `push` amortized O(1)). One that nothing else holds is
  changed in place.
- So a task that captures a struct shares every heap field of it at the
  spawn and releases them when it ends, and the first change on either
  side to a collection in it, while both hold it, copies that collection.
  A struct of hundreds of heap fields (the compiler's `Checker`) costs
  hundreds of count updates per spawn: capture the fields the task uses
  (`let part = c.field` before the spawn). `xo vet` notes such captures
  (`XO0534`), and a large parameter that is only stored (`XO0531`).
- `var b = xs[i]`, a change to `b`, then `xs[i] = b` copies the element at
  the first change, because `xs` still holds it; in a loop that is a copy
  per pass. Change `xs[i]` in place instead (`xs[i].push(v)`, decision
  0169). `xo vet` notes it for collection elements (`XO0536`); the Map
  form has `Map.update` (`XO0532`).
- A `Mutex.with` body changes the value while the mutex still holds it
  (an error or fault hands it back, decision 0085), so the body's first
  write to each collection in it copies that collection.

### 5.2 Mutation

- Only `var` bindings, `var` parameters, and `var` receivers can be mutated.
- `xs.push(v)` on a `var xs: List[T]` rebinds `xs` to a new persistent list
  sharing structure with the old one. The same holds for every mutating
  collection method (`put`, `remove`, `update`, `pop`, `add`).
- Field assignment `u.name = "b"` on a `var u` rebinds `u` to a modified copy.
  Nested paths work: `st.users.put(k, v)`, `st.config.port = 9090`.
- An assignment evaluates the indexes of its target, then the value, then
  stores into the target (decision 0140): changes the value makes to the
  target's binding are kept, so `t.objs[o].kids = t.add_kids()` keeps the
  objects `add_kids` pushed onto `t.objs`. A compound assignment
  (`+=`, `-=`, ...) reads the target's old value after the value, so
  `t.n += t.bump()` keeps what `bump` changed in `t`, `n` included
  (decision 0159).
- A call evaluates the place of a `var` receiver or `var` argument, then
  every argument in order, then reads those places (decision 0142):
  `t.get(t.add(v))` on a `var t` sees the item `add` pushed, and
  `xs.push(f())` keeps what `f` pushed onto `xs`. A receiver that is not
  `var` and is a variable or a field path (`t`, `t.items`) is read after
  the arguments too (decision 0326): `t.size(t.add(v))` with a by value
  `size` sees the item `add` pushed. Any other receiver (a call, an
  index) is evaluated first, in source order.
- To change a value stored in a collection, use `update`, which reads, mutates,
  and writes back in one call:
  ```
  let allowed = m.update(key, new_bucket(limit, now), fn(var b) { b.take(now) })
  ```
- **Lost update check.** Error `XO0520` ("mutation of `b` is never written
  back") applies to a local `var` whose initializer copies an element out of a
  collection (`m.get(k)`, `m[k]`, `xs[i]`, `xs.get(i)`, `first`, `last`,
  `find`, optionally followed by `??`, `?`, or `.or_err(...)?`) when the
  binding is mutated and is not stored again after its last mutation. Storing
  means passing the whole value as a call argument (`m.put(k, b)`,
  `save(b)`) or assigning it as a whole (`m[k] = b`). Reading or returning it
  does not count, because the collection still holds the old value. A plain
  `b = value` replaces the copy rather than changing it, so it is not a
  mutation (`l = f.values[l].args[0]` walking a chain). This
  catches `var b = m.get(k) ?? new_bucket(); b.take(now)` without
  `m.put(k, b)`, and a PATCH handler that returns the edited copy but never
  stores it. For an intentional scratch copy, copy through a local first
  (`let base = m.get(k)?; var scratch = base`). Values derived with `with`
  are not tracked, since views such as redacted copies are common. It also applies to any local `var` that is mutated after
  being stored into a collection (`xs.push(v)`, `m.put(k, v)`) and then not
  stored again, since the stored copy does not see the change. Reads inside
  `defer` count. Parameters, including `var` closure parameters, are never
  checked.
- **No aliased `var` arguments** (decision 0111). One call may not pass
  the same variable, or two overlapping places, to two `var` parameters
  (a `var` receiver counts as one): `both(xs, xs)`, `one(p, p.x)`,
  `p.add(p.y)` with a `var` receiver, and `both(g[i], g[j])` are
  `XO0503`. Places overlap when they start at the same binding and one
  path of fields and indexes is a prefix of the other; two different
  field names, or two indexes that are different integer or string
  literals (`g[0]`, `g[1]`), keep them apart. Copy one into a temporary
  first. With aliasing ruled out, passing by reference (the Go backend)
  and copying in and out (the native backends) give one result.

### 5.3 Move only buffers

`Buf[T]`, `StrBuilder`, and `BytesBuilder` are mutable in place and **move
only** (affine):

```
var b = Buf[Int].with_capacity(1024)
b.push(1)
let xs = b.freeze()      // moves b, returns an immutable List[Int] in O(1)
b.push(2)                // error XO0510: use of moved value `b`
```

- Assigning, passing, sending on a channel, or capturing in a spawned task moves
  the buffer. The compiler tracks moves only for these types and for the
  std handles whose declaration is documented `@move_only` (std/compress's
  Compressor and Decompressor, decision 0305). The marker is read only in
  std's declaration files: in user code it is an ordinary comment, so a
  user type cannot be made move only. There is no general borrow checker.
- A move on a path that ends (a `return`, a `break` or `continue`, the error
  side of `?`, a call of a function whose result is `Never`) does not reach the
  code after that point. A use is `XO0510` only when some path that can reach
  it moved the value and did not assign it again (decision 0300). A `break`
  carries its moves to the code after its loop and a `continue` to the loop's
  next iteration:

```
fn decode(s: Bytes) -> Bytes? {
  var out = BytesBuilder.new()
  for c in s {
    if c == 61 {
      return Some(out.freeze())   // the path ends: no move reaches below
    }
    out.write_byte(c)
  }
  Some(out.freeze())
}
```

### 5.4 Shared mutable state

```
let cache = Mutex[Map[Str, User]].new({})
cache.with(fn(var m) { m.put(key, user) })
let n = cache.with(fn(m) { m.len() })               // read only access
let size = cache.with_read(fn(m) { m.len() })       // shared with other readers
let hits = Atomic[Int].new(0)
let now_hits = hits.add(1)                           // returns the new value
```

`Mutex[T].with` has the signature

```
fn (m: Mutex[T]) with[R, E, fx: Effects](f: fn(var T) -> R ! E uses fx) -> R ! E uses fx
```

- The closure's return value (or error) is the result of `with`.
- Mutations to the `var` parameter are written back when the closure returns
  successfully. If it returns an error or faults, the protected value is left
  unchanged.
- A closure parameter declared without `var` gets a read only view.
- Warning `XO0540`: the closure writes its `var` value and a later path can
  return `Err` (`return Err(...)`, `?`, or a call that can fail as its
  result), so the write is undone. A sticky error is the common case:
  ```
  m.with(fn(var st) -> () ! E {
    st.err = Some(e)          // undone by the Err below
    Err(e)                    // XO0540
  })
  ```
- To keep changes and still report failure, return the outcome as the
  closure's success value and apply `?` outside:
  ```
  let outcome = q.state.with(fn(var st) -> Result[Job, QueueErr] {
    st.expire_leases(now)                  // kept
    if !st.token_valid(id, token) { return Err(QueueErr.StaleToken) }
    Ok(st.ack(id))
  })
  outcome?
  ```
- Warning `XO0804` (split critical section): a value read inside one
  `m.with(...)`, or derived from one, is used inside a later `m.with(...)` on
  the same mutex, or a branch on it decides whether the later `with` runs.
  Another task may have changed the state in between (check then act). Do the
  check and the action in one `with`.
- `with` cannot be called re entrantly on the same mutex within one task
  (fault `XO-F004` with both acquisition sites).
- `m.with_read(f)` (std.md 2.7) is shared read only access: `f` gets the
  value read only, readers run together, and `with` excludes them. Writers
  are preferred, so a reader must not wait for a reader that has not
  entered yet. A value read in `with_read` and used in a later `with` gets
  `XO0804` like one read in `with`.

### 5.5 Memory management

Tracing GC, plus request scoped arenas (decision 0066; on by default, off
with `--no-arenas`). An arena is memory owned by one HTTP request served by
`http.server` (std.md 4.4) and reused by a later request once the handler has
returned and the response is written. The compiler and the runtime place in it
only memory that provably cannot outlive the request: the request's header,
parameter, and query tables and its body when the handler uses the request
only through `json`, `param`, `param_int`, `query`, `header`, and its `Str`
fields (and not in a nested closure); the response built by `http.json` or
`http.text` that is directly the handler's result; and the `List` made by
`m.values()`, `m.keys()`, `m.entries()`, or `s.to_list()` when it is only
consumed in its own expression. Everything else, including every value a
program stores, sends, captures, returns, or hands to Go, is ordinary garbage
collected memory, so arenas are invisible: a program prints the same with and
without them. A user written `arena scope` block remains **[P2]**.

---

## 6. Effects and capabilities [P1 checking, P2 replay]

### 6.1 Effect labels

The builtin effects are:

| Effect | Sub effects | Authority |
|---|---|---|
| `net` | `net.dial`, `net.listen` | Sockets, HTTP, DNS |
| `fs` | `fs.read`, `fs.write` | Files and directories |
| `clock` | | Wall clock, timers, sleep |
| `rand` | | Randomness (crypto and math) |
| `env` | | Environment variables, args |
| `proc` | | Spawn processes, signals, exit |
| `stdio` | | Standard input, output, error |
| `ffi` | | Calls into C through `use c` (4.5, native builds) |
| `log` | | Writing a `std/log` record (`debug`, `info`, `warn`, `error`) |

A function with no `uses` clause is **pure**: it performs no capability
effects. It may still allocate, fault, be cancelled, and use
synchronized state (`Mutex`, `Atomic`, `Chan`).

Logging (`std/log`) performs the `log` effect (decision 0156): a `pub`
function that logs declares `uses log`, so a dependency's logging is
visible in its signatures, a ceiling can forbid it, the effect lock
catches an update that starts logging, and `xo audit` lists it. `log`
needs no capability (there is nothing to pass from `main`); it is
inferred for non `pub` functions like any effect, so `main` and helpers
are unchanged. `log.configure` performs `stdio`, not `log`. std's own
records (a handler fault, an `ignore_err`) stay attributed to std and do
not make std functions perform `log`. Log output goes to the sink
configured in `main` (standard error by default) and is recorded during
record and replay like any other output. Each record a required
module's code writes names it (decision 0173): a text record reads
`<time> <LEVEL> [<module>@<version>] <msg> k=v ...`, and a JSON record
carries the key `"module"` (which replaces a field of that name, so a
dependency cannot attribute its record to another module); `log.Record`
in tests has `module` (`""` for the main module). The main module's
records are unchanged. The call's source file decides, so a main module
closure run by a dependency stays the main module's.

Effect labels live in their own namespace: a parameter or module named `fs` or
`net` does not conflict with the effect `fs` or `net`.

### 6.2 The uses clause, effect variables, and subsumption

```
pub fn sync(src: Fs, out: Stdio) -> () ! SyncErr uses fs.read, stdio { ... }
```

- A function's effects are the union of the effects of everything it calls.
- `pub` functions and interface methods are not inferred: their `uses` clause
  is the declared set, and no clause means pure. Non pub functions and
  closures have their effects inferred.
- Performing an effect outside the declared set is error `XO0701`, which shows
  the call chain that performs it. Declaring more is allowed (for interface
  stability) and noted by `xo vet`.
- **Function types** carry effects. One effect: `fn(Int) -> Int uses clock`.
  Several effects are parenthesized: `fn(Int) -> Int uses (net, clock)`.
- In a declaration clause the parentheses are optional:
  `uses net, clock` and `uses (net, clock)` are the same.
- **Subsumption.** A function value is accepted where a function type is
  expected if its effects are a subset of the expected effects (`fs.read` is a
  subset of `fs`) and its error type converts to the expected one. A function
  that cannot fail is accepted where a failing one is expected. A pure function
  is accepted everywhere a function of the same shape is.
- **Effect variables** let a higher order function pass through its callback's
  effects. Declare them in the type parameter list with the `Effects` kind:
  ```
  pub fn run_pool[T, R, E, fx: Effects](
    jobs: List[T],
    workers: Int,
    f: fn(T) -> R ! E uses fx,
  ) -> Report[R, E] uses fx { ... }

  pub fn retry[T, E, fx: Effects](clock: Clock, f: fn() -> T ! E uses fx) -> T ! E
    uses (clock, fx) { ... }
  ```
  An effect variable is instantiated at each call from the argument's effects,
  and may be empty. When one effect variable is the effects of several
  function parameters, it is the union of the effects of those arguments
  (decision 0432). Elsewhere (an effect argument of a type, `Cache[K, V, fx]`)
  the variable is bound exactly, and a conflict is an error.
- **Structs may take effect parameters** (`struct Cache[K, V, fx: Effects]`),
  and effect sets may be written as type arguments (`Cache[Int, Item, net.dial]`).
- **Effect arguments may be omitted.** Writing `Cache[Int, Item]` leaves the
  effect arguments to inference. Either all of a type's effect arguments are
  written or all are omitted.
  - In a local binding they are inferred from the value.
  - In parameter and receiver types they become implicit effect variables of
    the function. These count as declared: the function may perform them
    without naming them in `uses`.
  - In result and error types they are inferred from the function body, so a
    constructor `fn new_cache(...) -> Cache[Int, Item]` returns the effects of
    the loader it was given.
  - In a struct field they become implicit effect parameters of the struct.
- A non pub function with a `uses` clause is checked against it. Test blocks
  may perform any effect on their `os`.

### 6.3 Capability values

Effects describe what a function may do. Capability values grant the authority
to do it. The only source of capabilities is `main`:

```
fn main(os: Os) ! {
  let srv = http.server(os.net.listen(":8080")?, router(os.net))
  srv.serve()?
}
```

`Os` has fields `net: Net`, `fs: Fs`, `clock: Clock`, `rand: Rand`, `env: Env`,
`proc: Proc`, `stdio: Stdio`, and `tasks: task.Group` (program lifetime task
group, core 7.1). Each capability type is a predeclared interface,
so test fakes and attenuated versions satisfy it. Their methods are in
`spec/std.md`.

`os.fs` is rooted at the filesystem root and accepts absolute paths
(`/home/me/a`, `C:/data`). Relative paths given to `os.fs` resolve against the
working directory. Capabilities can be attenuated. An attenuated capability
has the same type; operations outside its authority fail with a `Denied`
error. Inside a sub capability, paths are relative to its root:

```
let uploads = os.fs.sub("/var/app/uploads").read_only()
let api = os.net.only(["api.example.com"])
let config = os.env.only(["APP_PORT", "APP_DB_URL"])
let tools = os.proc.only(["git"])
```

`Net.only`, `Env.only`, and `Proc.only` (std.md 3.2, 3.5, 3.7, decision
0156) narrow the network to hosts, the environment to variables, and
processes to commands; a denied operation fails before any effect.
`xo audit` (10.1) lists the attenuations whose arguments are constants
and whether each capability passed into a dependency is attenuated.

A library cannot open a file or a socket unless it is handed a capability. A
dependency's reach is visible in its signatures.

### 6.4 Tests and fakes

See section 9.2: every test block receives `os: TestOs` with fake capabilities.

### 6.5 Real-time functions [native enforcement]

```
const TAPS = 32

struct Filter { coeffs: List[Float], window: List[Float], pos: Int }

fn (var f: Filter) step(x: Float) -> Float uses realtime {
  f.window[f.pos] = x
  f.pos = (f.pos + 1) % TAPS
  var acc = 0.0
  for i in 0..TAPS {
    acc += f.coeffs[i] * f.window[(f.pos + i) % TAPS]
  }
  acc
}
```

`uses realtime` on a function or method declaration marks it
**real-time** (decision 0133). `realtime` is written in the uses clause but
is a discipline, not an effect: it is not part of the effect set and
plays no part in subsumption. A real-time function has a declared effect
set like any function with a uses clause; the only effect it may declare
is `ffi`. `realtime` in a closure, a function type, an interface method,
or a test is `XO0702`. In a real-time function, including its contract
clauses:

- **No allocation** (`XO1401`): no List, Map, or Set literal, string
  interpolation, `+` or `+=` on Str, List, or Bytes, slicing, closure,
  store into a Map, or prelude method outside the bounded set: scalar
  methods of Int, the sized integers, Float, Rune, Duration, and Time;
  `len`, `is_empty`, `get`, `first`, `last`, `contains`, `index_of`, and
  `set` (also `xs[i] = v`) on List; `len`, `is_empty`, `get`, `contains`,
  `get_or_fault` on Map and Set; `is_some`, `is_none`, `ok`, `err`,
  `or_fault` on Option and Result; the Atomic operations; `len` on Str,
  Bytes, Chan, Buf, and the builders; `contains`, `starts_with`,
  `ends_with`, `find` on Str. The arguments of `fault(...)` are exempt.
- **Bounded loops** (`XO1402`): every loop is a `for` over a range with
  constant ends or has `bound N` (3.5). `for (i, x) in xs.indexed()`
  does not build the pairs and is allowed with a bound.
- **Known callees** (`XO1403`): calls go only to real-time functions, the
  bounded prelude methods, and functions of a `use c` module, never to a
  function value or an interface method.
- **No recursion** (`XO1404`): the calls among real-time functions form
  no cycle, so the stack depth is bounded.
- **No blocking** (`XO1405`): no channel operation (or `for` over a
  channel), Mutex, `scope`, `select`, task, `dbg`, capability call, or
  declared effect other than `ffi`.

Faults stay allowed (they are bounded checks); a fault ends the section.

**Initialization** is all code that is not real-time: it allocates
freely and prepares what real-time code writes in place, typically
Lists of a fixed length written by index (`List.set` is in place on a
List nothing else references, decision 0046).

**Real-time sections** [native builds]. A call from code that is not
real-time to a real-time function is a real-time section. Inside it the
native runtime:

- defers frees: a reference count that reaches zero puts the value on a
  per-thread queue of 256 entries instead of freeing it (and what it
  holds); the queue is freed when the section ends. A full queue frees
  at once (counted in `XO_RTSTATS=1` output as `rt_overflow`).
- counts allocations; a section that allocated faults when it ends with
  `XO-F007` (``real-time function `f` allocated n times``), which catches
  what the rules cannot see: a shared List copied on write, the box of a
  recursive enum, a defaulted field with a non-empty collection (an
  empty `[]` allocates nothing until its first push on the native
  backends).

The Go backend checks the rules and the loop bounds; it has no sections.

---

## 7. Concurrency [P1 basic, P2 deterministic]

### 7.1 Scopes and tasks

```
let page = scope s {
  let a = s.spawn(fn() { fetch_user(db, id) })
  let b = s.spawn(fn() { fetch_orders(db, id) })
  render(a.await?, b.await?)
}
```

- A scope is an expression; its value is the body's value.
- `s.spawn(f)` starts a **foreground** task and returns a `Task[T, E]` handle.
  The handle may be discarded.
- `s.spawn_background(f)` starts a **background** task (a daemon such as a
  cache evictor). Background tasks are cancelled automatically when the body
  finishes.
- `t.await` waits for a task and yields `T ! E`.
- **Exit rules.** When the body completes normally, the scope cancels its
  background tasks and waits for all tasks to finish. When the body exits
  early (`return`, `?`, `break`, error, or fault), the scope cancels all tasks
  and waits for them to unwind. No task ever outlives its scope.
- **Child failure.** If a task fails with an error that is not consumed by an
  `await`, or faults, the scope cancels everything else and the error
  propagates out of the scope as if by `?`: it must convert to the enclosing
  function's error type (error `XO0801` otherwise). A task that fails only
  because it was cancelled does not count as a failure.
- `s.cancel()` cancels all tasks of the scope (not the body).
- **Task groups.** `s.group()` returns a `task.Group`, a handle that can be
  stored in a struct or passed to functions so code can start tasks later
  (`g.spawn(f)`, `g.spawn_background(f)`). Tasks in a group belong to the
  scope that created it and never outlive it. A value of a type that contains
  a `Group` cannot be returned from, or stored outside, the creating scope
  (error `XO0802`). `main` receives `os.tasks`, a group that lives for the
  whole program, for services such as caches that refresh in the background.
  Tasks started on a group must not fail with an error the creating scope
  cannot convert (`XO0801`); a failing group task fails that scope. When the
  group's origin is not visible (it came from a field or parameter), a task
  started on it must not fail unless its handle is bound and awaited.
- The escape rule applies to values derived from that scope's `s.group()`.
  `os.tasks` lives for the whole program and may be stored anywhere.
- A task's error is consumed only if its handle is bound with `let` and
  awaited.
- Handles from an enclosing scope may be awaited inside `task.with_timeout`.
- Closures passed to `spawn` capture by value. Move only values are moved.
- The scope tree is the context: deadlines, cancellation, request ids, and
  trace spans flow down implicitly. There is no `ctx` parameter.
- Deadlines are set with `task.with_timeout` (see `spec/std.md`), which runs a
  closure in a child scope and returns `T ! Timed[E]`.

### 7.2 Cancellation

- Cancellation is **sticky**: once a task is cancelled, every cancellation
  point it reaches reports cancellation.
- Blocking operations are cancellation points: effect calls, channel
  operations, `await`, `clock.sleep`, `Mutex.with` and `Mutex.with_read`
  while waiting.
- A blocking operation that already returns a `Result` reports cancellation
  as its `Canceled` error variant (`http.ClientErr.Canceled`,
  `FsErr.Canceled`, ...). Code may map it to its own error, for example
  `FetchErr.Cancelled`, but cannot continue doing blocking work.
- A blocking operation that cannot fail (`clock.sleep`, `ch.send`, `ch.recv`,
  `t.await` in a cancelled task) unwinds the task immediately, running
  `defer` blocks.
- `clock.sleep_or_cancel(d) -> () ! ClockErr` is the sleep that reports
  cancellation as a value: when the task is cancelled before or during the
  sleep it returns `Err(ClockErr.Canceled)` instead of unwinding, so code
  that must return its own error (a retry loop's `FetchErr.Cancelled`)
  can. Cancellation stays sticky: the task's next cancellation point
  reports it again. Inside deferred code it sleeps in full, like `sleep`
  (decision 0088).
- Reading standard input (`stdio.read_line`, `read_all`, `lines`) is a
  cancellation point also while it waits for input: a cancelled reader
  gets `Err(IoErr.Canceled)` at once, and no input is lost (a line that
  arrives later goes to the next read).
- `task.is_canceled() -> Bool` reports the current task's state without
  blocking.
- Under a resource budget (`task.with_budget`, std.md 7, decision 0173)
  every loop iteration and every call into user code is a cancellation
  point too: a passed ceiling cancels the budgeted scope, and its tasks
  unwind at their next one.
- `defer` blocks run shielded (3.5).

### 7.3 Channels

```
let ch = Chan[Job].new(cap: 64)
ch.send(job)                // blocks when full
let j = ch.recv()           // Job? : None when closed and drained
ch.close()

select {
  j = jobs.recv() => handle(j)
  results.send(r) => count += 1
  after 5s => log.warn("idle")
}
```

- `ch.try_send(x) -> Bool` and `ch.try_recv() -> T?` never block.
- A `select` receive arm `v = ch.recv() => ...` binds `v` to what `recv`
  returns, a `T?`: a closed and drained channel's arm is ready at once
  and runs with `None` (so `j` in the example is a `Job?`; a loop over a
  `select` stops on `None` itself).
- `for x in ch { ... }` receives until `ch` is closed and drained; each receive
  is a cancellation point.
- `after` arms require the `clock` effect.
- Sending moves move only values.

### 7.4 Deterministic scheduling [P2]

Every `xo test` runs its tasks on a deterministic scheduler, seeded with
`--seed=N` (default 1): one task runs at a time, and at each scheduling point
the next task is chosen from the seed, so the same seed always produces the
same interleaving. Scheduling points are channel operations (including
`select`, `try_*`, and `close`), `await`, `Mutex.with`, `Mutex.with_read`,
`Semaphore.with`,
`Once.get`, `spawn`, `Atomic` operations, `sleep`, `task.is_canceled()`,
`task.stop_requested()`, and effect calls. Failing seeds are printed and can
be pinned with `test "x" seed 8812`.

`xo test --explore=N` reruns each concurrent test that passed under N further
seeds and reports the first failing seed. `xo run --seed=N` (or `XO_SEED=N` for
a binary the Go backend built, or a native one built with `--seed`) uses
the same scheduler with the real clock; interleavings
are then reproducible as long as timer and I/O timing does not decide them.

An Xo function that Go calls on a goroutine of its own (`use go`, for
example `sync.WaitGroup.go_`) runs as a scheduled task: Go's goroutine waits
until the scheduler admits the call, which happens only at a scheduling
point after Go's goroutines have settled, in a fixed order (by callback,
then arguments), so the seed decides how callbacks interleave. Once an Xo
function has crossed into Go, the return of every Go call is a scheduling
point. Go code woken by a Go timer or I/O of its own is not reproducible
(decision 0073).

Native builds (`--backend=arm64`, `amd64` and `llvm`) run tasks on the real
scheduler, on a pool of threads, so a native program whose output depends on
how its tasks interleave may print differently from run to run. `xo run
--seed=N` and `xo build --seed=N` build the program for the deterministic
scheduler instead (decision 0350): one thread runs every task, and at each
scheduling point a generator seeded with N (`XO_SEED=N` when the built program
runs, default 1) picks the next ready task, so one seed gives one
interleaving. Its scheduling points are the runtime's: channel operations,
`select`, `Mutex.with` and `with_read`, `spawn`, `await`, `Atomic` operations
and `task.is_canceled()`, and every wait; effect calls are not yet. A program
built without `--seed` ignores `XO_SEED`. Native `xo test` uses the
deterministic scheduler when given `--seed=N` or `--explore=N`, or when a test
pins its seed (`test "x" seed N`): each test runs with its own seed, else
`--seed`'s, else 1, and `--explore=N` runs a test without its own seed under
seeds 1 to N until one fails, reporting how to reproduce and pin it. The
seeds that expose a race differ from the Go backend's (its generator and
scheduling points differ). Other native tests still run on the thread pool
until decision 0350's stage 3; `xo run --record` still uses the Go backend.
The rest of core 7 means the same on every backend.

### 7.5 Process signals and graceful shutdown

Shutdown has two phases, so programs can finish queued work without fighting
sticky cancellation:

1. **Stop requested.** The first SIGINT or SIGTERM cancels nothing. It sets
   the program's stop state: `task.stop_requested()` returns `true` and the
   channel `task.stopping()` is closed, so loops can `select` on it. Servers
   started with `serve` stop accepting new connections and finish in flight
   requests. Readers stop reading: a read of standard input that is
   blocked, or starts later, returns `Err(IoErr.Canceled)` (decision 0088).
   Workers drain what is queued.
2. **Cancel.** When `main`'s body has not returned after the grace period
   (default 30s, set with `os.proc.set_grace(d)`), or on a second signal,
   `main`'s root scope is cancelled as in 7.2: blocking calls unwind and
   `defer` blocks run. A third signal exits immediately with status 130.

A program that ignores phase 1 behaves as before, only 30 seconds later; a
program that wants the old behavior calls `os.proc.set_grace(0s)`.

```
fn reader(os: Os, queue: Chan[Str]) -> () ! IoErr {
  defer queue.close()              // workers drain the queue, then exit
  while {
    match os.stdio.read_line() {
      Ok(Some(line)) => queue.send(line)
      Ok(None) => return Ok(())      // end of input
      Err(Canceled) => return Ok(()) // stop requested (or cancelled)
      Err(e) => return Err(e)
    }
  }
  Ok(())
}
```

A program can take SIGINT and SIGTERM over with `os.proc.notify` (std.md
3.7, decision 0407): while it is subscribed, that signal goes to its
channel instead of starting phase 1, and `os.proc.request_stop()` starts
phase 1 when the program chooses.

In tests, `os.proc.signal_after(d)` delivers the first signal to a program run
by `testing.run_main`; `os.proc.signal_after(d)` called again delivers the
next one.

Under `testing.run_main`, a third signal makes `run_main` return 130 after
`main` has unwound (the test process itself does not exit), and requests
handled in memory by a fake network observe the stop state but are not
cancelled by it.


---

## 8. Faults [P1]

A **fault** is a bug, not an error: overflow, out of bounds, failed contract,
failed `Int32(x)` conversion, evaluated hole, explicit `fault("msg")`.
`fault` has type `Never`.

- A fault terminates the current task. Its scope treats it like a failed child
  and cancels siblings. A fault reaching `main` exits the process with status
  70 and writes a crash bundle (section 11.2).
- `task.isolate(f) -> T ! Isolated[E]` runs `f` in a child scope and converts a
  fault into a value. `std/http` isolates every request and returns 500.
- Every fault message includes the evaluated values:
  `index 7 out of range for List of length 5 at users.xo:41:18`.
- A fault reaching `main` is reported on standard error as one line,
  `fault: MESSAGE at FILE:LINE:COL` (and a detail line for a failed
  contract). Native builds follow it with `traceback:` and a line per
  frame, innermost first, `  FUNCTION (FILE:LINE)` down to `main` (the
  line from the program's DWARF line tables, decision 0239; inlined calls
  show the caller's line), by default since 2026-10-22 (owner; a hardware
  fault's report has its frames too, decision 0343); `XO_TRACEBACK=0`
  leaves the frames out. A build with no function symbols left (`-s`, or a
  program stripped after the link) prints
  `traceback: (no function symbols: built with -s, or stripped)` instead
  of frames it cannot name. With `XO_TRACEBACK=locals` each frame is followed by its
  variables, `      NAME = VALUE` (decision 0268: from the program's
  DWARF, read at the fault; `<unavailable>` where the frame does not
  keep the value; numbers, `Bool`, `Str` text, a list's first elements
  (of any element type), structs' fields, an enum's variant with its
  fields, and `Os` by its type; a local declared at or after the frame's
  line with no value yet is left out).
- The native backends unwind a fault the same way (decision 0085: deferred
  code, scopes, Mutexes, and references are released on the way); they
  print the fault without its frames, write no crash bundle, and
  `Fault.frames` of `task.isolate` is empty there.
- A hardware fault (an access violation, a bus error, an illegal
  instruction, a trap) is not a fault: it is a bug below the language or
  in C code behind `use c`, and nothing unwinds. A native program on
  Linux, macOS or windows/amd64 reports it on standard error (decision
  0343): `fatal error: SIGNAL: TEXT (code N)`, with `at address A` for
  SIGSEGV and SIGBUS (Windows: `fatal error: EXCEPTION_NAME: TEXT (code
  0xNTSTATUS, read|write|execute) at address A` for an access
  violation), then the build information's line (`build: xo VERSION,
  BACKEND, TARGET, id ID, debug|release (SETTINGS)`, decision 0354), the
  pc and the image's load address, the registers,
  the frames' pcs (`frames:`) and the `traceback:` above; then the
  signal's default action ends the program, with the same exit status
  and core file as without the report (Windows: the same exit code, the
  NTSTATUS; an exception C code's own handler takes is not reported).
  `XO_CRASH_REPORT=0` turns it off; `XO_CRASH_TIMEOUT` (seconds, 5 by
  default) bounds the naming of the frames. Sanitizer builds report
  through their own runtimes; windows/arm64 does not report yet.

There is no undefined behavior. Debug and release builds have identical
semantics; release builds may only disable contract checks that are explicitly
marked `requires(debug)` or `ensures(debug)`.

- `xo build --release` is a release build (decisions 0099, 0354): a
  preset of the settings below. Without it every setting starts at its
  debug value. Any setting can be given either way in any build (`xo
  build`, `xo run`, `xo test`), before or after `--release`; an explicit
  flag wins over the preset, and between two explicit flags the last
  wins. A bare `--NAME` means on; booleans accept `on|off` and
  `true|false`.

  | flag | values | debug | release |
  |---|---|---|---|
  | `--debug-contracts` | on, off | on | off |
  | `--dbg` | on, off | on | off (`dbg(x)` is `x`, 11.4) |
  | `--error-args` | on, off | on | on (11.1) |
  | `--debug-info` | on, off, split | on | off |
  | `--profile-support` | on, off | on | off (XO_PROF, decision 0238) |
  | `--trace-support` | on, off | on | off (XO_TRACE, decision 0240) |
  | `--opt` | O0, O1, O2, O3 | the LLVM split's | O2 |

  `--debug-contracts=off` compiles the clauses marked `(debug)` out on
  every backend, `requires(debug) c else E` included (the function no
  longer returns that error); every other contract, every invariant,
  and every fault check stays. The other settings change what a build
  records or which diagnostic tools it carries, never what a program
  computes. `--opt` applies to the LLVM backend; with arm64 or amd64 it
  is a usage error, and the Go backend ignores it with a note.
  `--debug-info=split` (decision 0355) writes the debug information
  beside the program instead of in it: PROG.dSYM on darwin (matched by
  the Mach-O UUID), PROG.debug on linux (Xo's ELF linker: a GNU build id
  in both files and a `.gnu_debuglink` in the program); tracebacks,
  `xo dev symbolize`, debuggers and symbolizers find it there. It is an
  error on linux with a cc or clang link (`use c`, `--ld=cc`, the LLVM
  backend), on wasip1, and with the Go backend. A release build fails while an xo.work workspace
  overrides a required module (`XO1308`, 4.2); no flag overrides that.
- Every other build is a debug build: `xo build` without `--release`,
  `xo run`, `xo test` (whose contract fuzzing checks the debug clauses
  too), and `xo replay`, which rebuilds with the settings recorded in
  the program's build information. Every binary records the effective
  value of each setting, native ones included (`xo version -m BINARY`
  prints them; a native crash report's `build:` line names them). `--ci` is a gate, not a mode: it does not
  imply `--release`; a pipeline that ships the binary passes both.

---

## 9. Contracts and tests [P1]

### 9.1 Contracts

```
fn transfer(var from: Account, var to: Account, amt: Money) -> () ! TxErr
  requires amt > 0
  requires from.balance >= amt else TxErr.Insufficient
  ensures from.balance + to.balance == old(from.balance + to.balance)
{ ... }

struct Account { id: Int, balance: Money } invariant balance >= 0
```

- `requires expr` faults on violation. `requires expr else E` returns `Err(E)`
  instead, which makes input validation declarative. A function may consist
  only of contracts and an empty body.
- `ensures` may reference `result` (the success value) and `old(expr)`.
- `invariant` is checked after construction (including from defaults) and
  after every `var` method.
- Contract failures print every sub expression's value:
  ```
  ensures failed: from.balance + to.balance == old(from.balance + to.balance)
    from.balance = 40   to.balance = 50   old(...) = 100
  ```
- `xo test` fuzzes every function with contracts using generated inputs that
  satisfy `requires`, and checks `ensures`. Capability parameters receive
  seeded fakes. Contract clauses must be pure.

### 9.2 Tests

```
test "fetch missing user" {
  let store = new_store()
  expect(store.get(7)) == Err(LoadErr.NotFound(id: 7))
}

test "reverse twice" for xs: List[Int] {
  expect(xs.reverse().reverse()) == xs
}

test "retries after 503" {
  let calls = Atomic[Int].new(0)
  os.net.handle("GET https://api.test/x", fn(_req) {
    if calls.add(1) < 3 { http.empty(503) } else { http.text(200, "ok") }
  })
  let body = get_with_retry(os.net, os.clock, os.rand, "https://api.test/x")?
  expect(body) == b"ok"
  expect(os.clock.sleeps()) == [100ms, 200ms]
}
```

- `test` blocks may live in any file. They are compiled only by `xo test`.
- `xo build` and `xo run` (with `--watch` too) do not read `_test.xo`
  files at all (decision 0348): their declarations, errors, and `use`
  lines do not exist for a build, so a helper declared only there is
  `XO0601` in production code and a test-only import is never fetched
  or bound. `xo test`, `xo check`, `xo vet`, `xo fix`, the language
  server, and the lookup tools read them.
- `xo build` and `xo run` skip inline `test` blocks the same way
  (decision 0389): a block is neither declared nor type-checked, so an
  error in it does not fail a build, a helper or closure declared in it
  does not exist for the build, and a build does not load `std/testing`
  for it. `xo test`, `xo check`, `xo vet`, `xo fix`, and the language
  server check every test block.
- Where tests live (a note, not a rule). Both places have the same access
  to the module's private names; they differ in what else sees the edit:
  - Inline `test` blocks keep a test next to the code it checks, so a
    reader sees it as an example and an edit to the code updates it in
    the same place; they suit small modules, examples, and one-file tools.
  - A `_test.xo` file keeps tests out of the files the program is built
    from. A tool that rebuilds or restarts on file changes (a file
    watcher, a cached build of the program keyed by its source files)
    then skips test-only edits by name, which it cannot do for an inline
    test. Large modules and long running servers usually keep
    their tests there. In the Xo repository, tests under `std/` and
    `compiler/` live only in `_test.xo` files (decision 0262), because
    the cached xo is keyed by those sources. `xo run --watch` needs no
    such exclusion by name: it compares the built program, so an edit
    that does not change the binary (a test, inline or not, a comment)
    never restarts it (decision 0267).
- A test body has type `() ! Error`, so `?` fails the test with the error.
- `opt?` on an Option directly in a test body (at any depth of blocks,
  `if`, `match`, and loops in it, property tests included) yields the
  value or fails the test as `opt.or_err("missing value: opt")?` would,
  `opt` being the source text of the operand (decision 0347):
  ```
  test "finds the user" {
    let u = store.find(7)?   // --- FAIL: ... then: error: missing value: store.find(7)
    expect(u.name) == "Ada"
  }
  ```
  An Option's `?` and a Result's `?` in the same test both fail it; their
  error types are not unified. A `?` inside a fn literal written in a
  test keeps that function's own rules (2.5), and a helper function in a
  `_test.xo` file is not a test body: `opt?` there is still `XO0204` or
  `XO0205`, with a note. A property test's missing value is a failed
  case, shrunk like any other.
- Inside a test block, and anywhere in a `_test.xo` file for `expect` and
  `testing`, three names are predeclared:
  - `os: TestOs` with fake capabilities (in memory filesystem, virtual clock,
    seeded rand, network that fails unless a handler is registered, captured
    stdio, fake env and proc). Fakes have extra control methods such as
    `os.clock.advance(d)` and `os.fs.fail_next(...)`; see `std/testing` in
    `spec/std.md`. Each fake satisfies the matching capability interface.
  - `expect(x)` followed by any comparison: `expect(a) == b`, `expect(n) <= 3`.
    `expect(cond)` alone asserts a `Bool`.
  - `testing`, the `std/testing` module.
- The virtual clock auto advances: when every task in the test is blocked,
  time jumps to the next timer, so sleeps and timeouts finish instantly.
- `for` on a test makes it a property test over generated values.
- Failures found by fuzzing or property tests are shrunk to a minimal input.
  A shrunk contract fuzz failure is written as a permanent test to
  `regressions_test.xo` in the module itself (so it can call private
  functions), named `regression: <function> (seed N)`, but only if the module
  still type checks with it; otherwise `xo test` says why and the seed
  reproduces the failure. Shrunk property test failures are reported with
  their seed and input.
- Real capabilities require `test "name" uses net { ... }` and
  `xo test --real`; `os` then holds real capabilities for the listed effects.
- Each test, and each case and shrinking step of a property test, may run
  for `xo test --timeout=D` of real time (default 60s; 0 for no limit).
  One that does not finish fails with `the test did not finish within D of
  real time` and its log records, and the next test runs; on the native
  backends the harness kills the test's process (decision 0235). The
  virtual clock does not count: only real work (a loop that never yields,
  a real capability that never answers) reaches the limit.

---

## 10. Toolchain and diagnostics [P1]

### 10.1 Commands

| Command | Purpose |
|---|---|
| `xo build [--targets=all\|os/arch,...] [--no-gc] [--tzdata=embedded\|system] [--ci] [--release] [--debug-contracts\|--dbg\|--error-args\|--profile-support\|--trace-support=on\|off] [--debug-info=on\|off\|split] [--opt=O0..O3] [--stamp NAME=VALUE]...` | Build static binaries; `--stamp` (repeatable, also `xo run`) sets a `const NAME: Str = "..."` of the main module to VALUE for this build, else `XO1501`, and is recorded in the build information so `xo replay` rebuilds the same program (decision 0165); `--no-gc` on a native backend without a garbage collector (below); `--tzdata=system` reads time zones from the host instead of embedding the ones the program uses (std.md 13); `--ci` fails before building on an unformatted file (10.5) or a `dbg` left in code (11.4); `--release` is the release preset (8, decision 0354): `requires(debug)` and `ensures(debug)` compiled out, `dbg` off, no debug information, the native runtime's profiler and execution trace recorder left out, `--opt=O2`; each of those settings has its own flag, in any build, that wins over the preset; `--error-args=off` drops parameter values from error frames (11.1); `-s` leaves debug information and local symbols out of a native binary. Builds are debug builds otherwise, and `xo run` and `xo test` always are |
| `xo run [DIR]` | Build and run for the host the program in exactly DIR (default `.`), never one below it (decision 0345); `--profile=cpu\|heap\|cpu,heap` (XO_PROF, decision 0238) and `--trace=FILE` (XO_TRACE, decision 0240) on a native backend; `--watch [--grace=D]` rebuilds after a source change and restarts the program only when the built binary changed (SIGTERM, up to `--grace`, default 5s, then SIGKILL; decision 0267). A stop request `xo run` gets (SIGINT or SIGTERM, 7.5) is sent on to the program as SIGTERM when standard input is not a terminal; on a terminal, Ctrl-C reaches the program through the process group |
| `xo trace FILE [--json OUT] [--text]` | The execution trace a native program wrote under XO_TRACE as Chrome trace event JSON (Perfetto) and a summary (decision 0240) |
| `xo check [--backend=go] [--support] [--fix]` | Parse and type check all targets, no codegen, by the rules of the backend a build uses: the native backends' by default (`use c` is valid), the Go backend's with `--backend=go`; `--support` (old name `--no-gc`) adds the native backends' support check (XO1201), which lowers the program; `--fix` first applies the machine fixes (below) |
| `xo test [--seed=N] [--explore=N] [--cases=N] [--timeout=D] [--real] [--watch]` | Tests, contract fuzzing, property tests (`--cases` per property test and fuzzed function, default 100; `--timeout` the real time each test may take, default 60s, 0 for none, 9.2); deterministic scheduling. `--watch` reruns, after each change, only the tests the change reaches (decision 0267). Doc comment examples are [P2] (1.2) |
| `xo dump <pid>` | Task tree of a running program (11.4) |
| `xo mcp` | Model Context Protocol server over stdio: `check` (with an optional `fix`), `query`, `edit`, `explain`, `fmt`, `test`, `build` tools returning compact JSON |
| `xo lsp` | Language Server Protocol server over stdio for editors: diagnostics with quick fixes, hover, definition, references, rename, document symbols, semantic tokens, formatting |
| `xo serve` | Resident checker on a unix socket; `xo check` and `xo query` use it automatically when it answers |
| `xo fmt` | Canonical format, no options |
| `xo fix [--dry-run] [--from=draft-0.N] [--edition=preview]` | Apply all machine fixes until none apply, xo.mod and xo.work lines included (`edition 2026` becomes `edition preview`, `xo 2026` becomes `xo preview`, `XO1315`); `--from` migrates code written for an older draft (interpolation, prelude variants, `let _ =` discards, scope timeouts, Map index reads) |
| `xo vet` | Advisory checks, never errors (a directory, a `.xo` file's findings, a std directory checked as std, or `--std-module=M`; decision 0289): checker warnings plus `XO0207` (`??` precedence trap), `XO0208` (each `ignore_err` with its reason), `XO0530` (a field's large payload bound and shared, decision 0263), `XO0531` (a large parameter only stored while each caller passes its last use, decision 0263), `XO0532` (a Map value changed through a copy and stored back, `Map.update` instead; decision 0266), `XO0534` (a task capturing a value of 32 or more heap fields, core 5.1; decision 0294), `XO0703` (declared effect never performed), `XO0901` (`dbg` left in code), `XO0902` (`tls.Verify.None`, decision 0281), `XO0903` (an Fs, Net, or Proc error defaulted away by `.ok() ?? d` or `Err(_) => <empty>`; `// xo:allow XO0903 <reason>` silences one), `XO0904` (an embedded file git ignores or does not track; decision 0304), `XO1103` (use of Go packages per module), `XO0535` (`${x.debug()}` written as `${x:?}`, with the fix; decision 0296), `XO0536` (a list element copied out, changed, and stored back; decision 0294), `XO0906` (a four line `match` that `x ?? return v` or `r ?? e => return v` writes shorter; decision 0321), `XO0538` (a closure's capture written after its creation). `--cost` (decision 0327) builds the program up to its reference counting and adds notes on costs its source does not show: `XO0907` (a loop that copies a map's entries first), `XO0908` (an element counted up and down each iteration, or by a prelude callback walk such as `xs.any(fn ...)` over elements holding heap memory), `XO0909` (a list updated in a loop while a holder made in the same iteration keeps it shared, so each update copies it), `XO0910` (a value over 8 words copied into a closure, or out of a collection into an Option in a loop), `XO0911` (a view of a parameter's text or bytes made in a loop, or an element of its split or lines, stored in a field or a returned collection: it keeps the whole input alive; `clone()` it, decision 0330). |
| `xo def`, `xo refs`, `xo type`, `xo doc NAME`, `xo symbols` | Agents' lookups on `xo lsp`'s engine (decision 0261): `def NAME`, `refs NAME`, `type FILE:LINE:COL`, `doc NAME` (a name, or a module path and name: `std/tls.client`), `symbols [DIR] [--pub]` (the directory's own declarations, private ones too); a name in any form `xo query def` takes, std and runtime modules included (decision 0289); `--dir` (a workspace root checks the modules under it); text at a terminal, `--json` or `--text` to choose; exit 0 with an answer, 1 for no match, 2 for usage |
| `xo query <kind> ...` | Semantic queries (10.3): `type`, `def`, `callers`, `effects`, `holes`, `api`, `imports`, `deps` |
| `xo edit <op> ...` | Semantic edits (10.4) |
| `xo replay`, `xo dump` | Debugging (section 11); `xo why` and `xo debug` are [P2] and not implemented yet (11.3, 11.5) |
| `xo doc` | Docs from doc comments |
| `xo clean -cache [-n]` | Remove every cache under `<user cache>/xo` (build directories, the native back end's cache, GOCACHE, test binaries, watch builds, xo dev outputs, the test infrastructure's caches) and print the bytes freed per cache; `-n` prints what it would remove and removes nothing. Never touches anything outside `<user cache>/xo`; the module cache and the prebuilt seeds stay; an entry whose build lock a running process holds is left. `xo clean` without `-cache` is a usage error |
| `xo get [--accept-effects] <path>[@version]`, `xo mod tidy`, `xo mod accept-effects [<path>...]`, `xo vendor`, `xo mod clean`, `xo mod proxy serve` | Dependencies (4.2, 4.3): require a module version, tidy xo.mod and xo.lock, record wider effects of dependencies in xo.lock (decision 0156), copy dependencies into vendor/, empty the module cache, serve a module cache as a module proxy (decision 0126). Every command takes `--offline` and `--workspace=off` |
| `xo audit [--json] [--no-gc] [<dir>]` | Supply chain report (decision 0156): per dependency its effects, its xo.lock effects line (ok, widened, narrowed, unlocked), its ceiling, and its escape hatches (`use c`, impure `use go`) with the vouches covering them; this module's attenuations (`only`, `sub`, `read_only`, `write_only`, `cwd`) with constant arguments, the capabilities it passes into dependencies and whether each is attenuated, vouches that match nothing, and the check's diagnostics. JSON with `--json` or when stdout is not a terminal (an agent piping it gets JSON), text otherwise. Exit 1 when the check has errors |
| `xo init <path>`, `xo publish [--check]` | Start a module (xo.mod, xo.work gitignored); check a release against the previous tag (4.2) |
| `xo explain <code>` | Long form explanation of a diagnostic |
| `xo version [--json \| -v]`, `xo version -m [--json] BINARY...` | The toolchain's version and host target; `--json` and `-v` add what xo was built from (decision 0264); `-m` prints the build information a program built by xo carries on every backend (decision 0354): the xo, backend and target, its ID and directory, every effective setting of 8 (and `--opt`), the stamps, and its sources' hashes, as `go version -m` does for Go |
| `xo bind [--json] <Go path>...`, `xo bind --c [--link x]... [--uses none\|e,...] <header>...` | Generate the bindings of Go packages (`use go`, 4.4) or C headers (`use c`, 4.5) into the project's binding cache |

The commands that report diagnostics or results accept `--json`: `parse`,
`check`, `build`, `test`, `fmt`, `fix`, `vet`, `query`, `edit` (always
JSON), `doc`, `explain`, `bind`, and `audit`. `run`, `replay`, `dump`, `serve`, and
`version` print the program's or the tool's own output. When stdout is not a
terminal, JSON is the default for `query`.

**Help and usage errors.** `xo help` lists every command with a one line
purpose; `xo help <command>`, and `-h` or `--help` anywhere before `--`
(for `xo run`, before its directory), print that command's usage on
stdout with exit status 0. `xo` alone prints the command list on stderr
and exits 2. An unknown flag, a missing argument, or an unknown command
is a usage error: the error and the usage on stderr, exit status 2.

**Directories and `DIR/...`.** A directory argument means that
directory alone (decision 0345, superseding 0210's implicit walk):
`check`, `vet`, `doc`, `test`, `build`, and `audit` take the module in
it (plus the modules it imports, and for `check` the other modules of
its `xo.work` workspace), and `fmt`, `fix`, and `parse` its own `.xo`
files. A directory with no `.xo` file is an error that names `DIR/...`.
Only the package pattern `DIR/...` (`./...`, `cmd/...`, accepted by
`check`, `test`, `build`, `vet`, `doc`, `fmt`, `fix`, `parse`, `audit`)
covers the directories below DIR: every directory with `.xo` files at or
below it (one module each, or one program for `build` and `test`),
or for `fmt`, `fix`, and `parse` every `.xo` file below it. Every such
walk skips directories named `testdata` or `vendor` (with or without
`modules.txt`) and names starting with `.` or `_`, as the go command
does, so a deliberately broken directory is kept out of `./...` by its
name; vendored code is checked only when its directory is named. A `DIR/...` walk stops past 6,000 `.xo` files or 60,000 directory
entries with an error naming the limit, so `xo build /...` or
`xo check $HOME/...` refuses. A `DIR/...` with no `.xo` file at or below
DIR (with the skips above) is an error, `xo CMD: DIR/... matched no
directory with .xo files`, exit status 1. Other forms (`...` alone,
`a/.../b`) are usage errors (decision 0210).

`xo run` takes one directory (`.` when none is given) and runs the
program in exactly that directory: it never looks below it, since
running code found by walking would run whatever a directory tree
holds. A directory with no `.xo` file or no `main` is an error, whose
note lists the subdirectories directly below that declare `main`, as a
hint; a `.xo` file, `xo run DIR/...`, and any other pattern are errors
(a module is a directory, 1.1), the file's with the fix `xo run DIR`.

**Check and fix.** `xo check` is read only. `xo check --fix` first
applies every unambiguous machine fix, the same set `xo fix` applies
(without `--from`), round after round until none applies, reformats the
files it changed, and then reports the diagnostics that remain, with the
exit status of `xo check` on them. Its JSON adds `applied`, the list of
fixes made (`{file, line, rule, title}`, `rule` being the diagnostic
code); plain `xo check --json` has no `applied` field. This is the agent
loop's check: an agent runs `xo check --fix` and then `xo test`, so a
diagnostic with an exact fix never costs a run (decision 0096). The MCP
`check` tool takes `fix: true` for the same.

**Backends.** `xo build`, `run`, and `test` compile natively by default
(decision 0248): the arm64 backend on arm64 hosts, the LLVM backend (which
needs clang) otherwise. `--backend=go` compiles through the Go toolchain
and is required by a program with a `use go` package; with no `--backend`
such a program is a usage error naming it. The flags only the Go backend
has (`--hybrid` and `xo run --record`) choose it when no backend is
given; `xo run --seed`, `xo test --seed` and `xo test --explore` build
natively for the deterministic scheduler (7.4, decision 0350). `--race` keeps the
native backend where a ThreadSanitizer runtime ships for the target
(darwin/arm64, darwin/amd64, linux/arm64, linux/amd64, windows/amd64)
and otherwise
takes the LLVM backend's (decision 0273 addendum). Contract fuzzing (9.2) runs on every backend
(decision 0250): the same targets, cases, and seeds, the same tests named
`contract fuzz: MODULE.F` (which `--run` matches like any test name), and
the same regression tests; a target whose generated native test does not
check is skipped and counted per module (`MODULE: contract fuzzing needs
--backend=go: N contracts not fuzzed`, the reason under `-v`). `xo dump`
(11.4) works on every backend (decision 0251).

**Builds without a garbage collector.** `xo build --no-gc` (also `xo run`,
`xo test`, and `xo check`) builds on a native backend whose memory is
reference counted instead of collected: arm64 on darwin/arm64 and
linux/arm64 hosts, LLVM otherwise; `--backend=llvm`, `--backend=arm64`, or
`--backend=amd64` (Xo's own x86_64 backend for linux/amd64, decision 0254)
picks one, and `--backend=go` with `--no-gc` is a usage error. After type
checking and before any code generation, a check reports every construct,
std API, or effect the native backends do not support as error `XO1201` at
its site, naming the feature and saying whether support is *planned* (build
without `--no-gc` until then) or *impossible* (code from a `use go` package
needs Go's runtime). A program the check accepts builds; the check is
derived from the native code generator itself, so the two cannot disagree.
`xo check --no-gc` runs the same check without building (programs only: a
module without `main` has nothing to lower). Native builds have their own
deterministic scheduler for `--seed` and `--explore` (7.4, decision 0350);
recording (`--record`) is the Go backend's only (decision 0083). `xo test --no-gc` builds a
native test binary and runs each test in a process of its own against the
fakes of 9.2 (virtual clock, seeded `Rand`, fake environment, streams, and
network, MemFs, FakeProc, `os.tasks`) with the same report as the Go
backend; a fault fails only its test.
Property tests run there with the Go backend's cases and shrunk inputs
(one process per case, decision 0120), `--real` gives real capabilities
for the declared effects (a test declaring `clock`, `net`, or `proc` runs
on the real clock there), contract fuzzing (9.1) is reported as skipped
(decision 0085), and error
traces are recorded natively but printed only in main's error report
(decision 0104), and fault frames are not recorded natively.

**Hybrid builds (deprecated).** Since 2026-10-22 (owner) `--hybrid` is a
plain Go backend build that warns that it is deprecated and will be
removed: compiler/arm64 has no Go assembler mode, so nothing below
happens. As decision 0084 described it, `xo build --hybrid` (Go backend)
compiled the hot leaf
functions of the main module (a loop over `Int`, `Float`, `Bool`, and
`List`s of those or of structs of `Int` and `Float` fields, with no
allocation) to native assembly linked into the Go program; everything
else, and every program the native backends cannot lower, stays Go.
Behavior is the Go backend's, faults included, except that fault frames
omit the assembly functions and `Float` results may differ in the last bit
where the Go backend fuses a multiply and an add (decision 0084). Loops
in assembly yield to the scheduler and the garbage collector as Go code
does (decision 0131).

### 10.2 Diagnostic format

```json
{
  "code": "XO0201",
  "severity": "error",
  "message": "result of `fetch_user` is not handled",
  "span": {"file": "api/users.xo", "line": 18, "col": 3, "end_line": 18, "end_col": 24},
  "notes": [{"message": "`fetch_user` can fail with LoadErr", "span": {"file": "users/fetch.xo", "line": 4, "col": 1}}],
  "fixes": [
    {"title": "propagate with ?", "edits": [{"span": {"file": "api/users.xo", "line": 18, "col": 24, "end_line": 18, "end_col": 24}, "replace": "?"}]}
  ]
}
```

- Codes are stable forever within an edition.
- Codes are grouped: `01xx` lexical and naming, `02xx` errors and results,
  `03xx` match, `04xx` types and generics, `05xx` mutation and moves,
  `06xx` resolution, `07xx` effects, `08xx` concurrency, `09xx` holes and
  contracts, `10xx` platform, `11xx` Go interop, `12xx` native builds
  without a garbage collector (`XO1201`: not available with `--no-gc`,
  planned or impossible; 10.1). Every code is listed in 10.6.
- The compiler reports all errors it can, not just the first, and never
  cascades: a bad expression has type `<error>` and suppresses dependent
  diagnostics.
- **Habits from other languages** (decision 0087). Code written with the
  syntax or library names of Go, Rust, TypeScript, Python, Swift, or
  Kotlin gets one diagnostic per habit that names the habit and the Xo
  form, with a machine fix when the rewrite is unambiguous, and checking
  continues as if the Xo form had been written:
  - Syntax that the grammar would otherwise reject with a generic code
    (`XO0160`, `XO0168`, `XO0172`, `XO0175`) is error `XO0177` (syntax
    from another language): for example `func`, `x := v`, `let mut`,
    `T: Hash + Eq`, `Vec<T>`, `[]T`, `var x` or `&x` at a call site,
    `c ? a : b`, `switch`/`case`, `when`, `elif`, `and`/`or`/`not`,
    `for i := 0; i < n; i++`, Go `for k, v := range m` (reported by the
    checker, which picks the Xo form for the type of `m`), `x++`, `throw`,
    `try f()`, `|x| e`, `(x) => e`, `lambda`, `x as T`, `A::B`, `impl`,
    `trait`, `): T` and Go's bare result types, `(T, error)` results,
    backtick templates, f-strings, and `/* */` comments. `xo explain
    XO0177` lists them.
  - When a specific code already describes the error, it keeps that code
    and gains the habit in a note and the fix: `XO0601` for names from
    other languages (`nil`, `null`, `this`, `len(x)`, `append`, `print`,
    `String`, `Vec`, `int`, `.unwrap()`, `.length`, camelCase methods,
    Python's `t = 0` declaring by assignment), `XO0169` (commas between
    match arms), `XO0173` (`use` after declarations), `XO0701` (an effect
    argument left out of a parameter type, which stays an abstract effect
    variable), `XO0408` (TypeScript's `const x = f()`), `XO0110` (Kotlin's
    `"$name"`, literal text in Xo), `XO0205` (`x?.f` optional chaining in
    a function that does not return an optional), `XO0403` (a missing
    `reason`), `XO0404` (JSON for tuples), `XO0101`, `XO0502`, and the lexical
    codes `XO0150` (`#` comments, `#[derive]`), `XO0152` (`'text'`), and
    `XO0153` (Swift's `\(x)`).
  - A habit gets a fix only when one rewrite keeps the meaning; where the
    Xo form needs a decision (`x!!` needs a reason, `try`/`catch` needs a
    Result handling choice, `print` needs a capability in scope) the
    diagnostic names the form and has no fix.
- A suggestion from edit distance (`did you mean`) is a note; it is also a
  fix only when it is clear: a unique closest candidate at most half the
  name's length away, for names of three or more characters.
- An unknown method or field suggests the receiver's own fields and
  methods: the closest by edit distance, also counting a candidate
  without its `to_`, `is_`, `as_`, or `get_` prefix one further away
  (`s.uppr()` suggests `to_upper`). A name another language uses for a
  method Xo has with the same meaning (Go's `TrimSpace` and `HasPrefix`,
  Rust's `chars` and a Map's `insert`, TypeScript's `some` and `every`)
  names that language and the Xo method, with the rename as the fix.
- A diagnostic's first fix is the one `xo fix` and `xo check --fix` apply
  (10.1); fixes whose edits overlap in one round wait for the next round.
  `xo check --fix --json` adds `"applied": [{"file", "line", "rule",
  "title"}]` next to `modules` and `diagnostics`.
- `xo build`, `xo run` and `xo test` print every error, but only the
  warnings of files in the directories the command names (and the
  directories below them); the others are counted on one line, and
  `XO_WARNINGS=all` prints them. A build of `compiler/xo` does not show
  `compiler/ssa`'s warnings although both are one module; `xo check` and
  `xo vet` on a directory print all of its warnings, and `--json` output
  holds every diagnostic.

### 10.3 Queries

```
xo query type api/users.xo:18:7
xo query def http.server
xo query callers users.fetch_user
xo query effects api.get_user          // with the call chain per effect
xo query holes
xo query api github.com/acme/billing   // exported surface of a module
xo query imports example.com/notes/domain   // its `use` lines
xo query deps example.com/notes/server      // the same, transitively
xo query hash users.fetch_user              // content hash of a definition (4.3)
```

`type` takes FILE:LINE:COL; with `--dir=DIR`, a relative FILE that is
not under the working directory but is under DIR names that file (`xo
mcp`'s query tool passes its `dir` this way).

`hash` (decision 0125) answers `{list}`, one entry per definition the
name resolves to (a name, `Type.method`, `pkg.name`, or `<package
path>.name` of the module holding `--dir`): `{id, package, name, kind,
pub, hash, own, refs, file, line}`. It needs no check run.

`imports` and `deps` (decision 0095) answer
`{package, transitive, imports, packages}`: each entry of `imports` is a
`use` line `{from, path, pos, std, go, test, missing, denied}`, where
`denied` is the `XO1312` message when a `deny` rule forbids the line, and
`packages` lists the distinct paths reached. `deps` follows every
imported package of the program (not std or Go packages).

### 10.4 Semantic edits [P1: rename, add-field, change-sig]

```
xo edit rename users.fetch_user users.load_user
xo edit add-field User created: Time = Time.UNIX_EPOCH
xo edit change-sig users.load_user "(db: Db, id: UserId) -> User ! LoadErr"
```

Edits update every reference atomically and are rejected if the result does
not type check.

- References are found through the checker's resolved symbols, never by
  text search. An edit of a program that does not check clean is rejected
  before anything is computed.
- `rename` renames a function, method, or constant and every call and use
  as a value, or a type and every place it is written (type expressions,
  struct literals, patterns, variants, receivers; decision 0289). The new
  name may repeat the target's qualifier. With `--dry-run` the summary
  carries a diff of the changes.
- `add-field` adds `name: Type` to the struct declaration and
  `name: default` to every struct literal of the type. The default is a
  migration value; it is not kept as a declared field default. Without
  `= default` the edit is rejected when struct literals exist.
- `change-sig` replaces the parameter list, result, and error type (not
  the clauses). Call arguments are matched to the new parameters by name;
  a parameter renamed in place keeps its position, and its uses in the body
  are renamed. New parameters with a default need no argument. A call that
  passes an argument for a removed parameter, or lacks one for a new
  parameter without a default, rejects the edit, listing those call sites.
  Arguments keep their order (and so their evaluation order); arguments
  that no longer match their position become named.
- The result is a JSON summary: `{op, target, ok, written, reason, files,
  edits, diagnostics, sites}`. `diagnostics` holds the errors that rejected
  the edit; `sites` the call sites or literals that blocked it. `--dry-run`
  checks the edit without writing.

### 10.5 Formatting

`xo fmt` produces one canonical layout from the AST. There are no options.
`xo build` fails on unformatted files in CI mode (`--ci`).

### 10.6 Diagnostic codes

Every code the toolchain reports, its default severity, and what reports
it: the lexer and parser (`xo parse` and every later command), the checker
(`xo check`), `xo vet` (advisory, never an error except `XO0901` under
`xo build --ci`), the no GC check (`--no-gc`, 10.1), and module
resolution (`modules`: every command that loads a program reports these
on the xo.mod, xo.lock, or xo.work line, and so do `xo get`, `xo mod
tidy`, and `xo vendor`; 4.2, 4.3), and `xo publish` (`publish`).
`xo explain <code>` prints the long form of each. The registry is
`compiler/cli/core/explain_data.xo` (internal/diag/codes.go until
decision 0215); a test (internal/repotest) keeps this table equal to it.

| Code | Severity | Reported by | Meaning |
|---|---|---|---|
| `XO0101` | error | checker | name breaks the enforced naming rules (core 1.5) |
| `XO0110` | error | checker | unused local binding or import (core 3.1) |
| `XO0150` | error | lexer | character that cannot start any token |
| `XO0151` | error | lexer | string, raw string, bytes, or triple quoted string not closed |
| `XO0152` | error | lexer | rune literal that is empty, unterminated, or holds more than one character |
| `XO0153` | error | lexer | unknown or malformed escape sequence |
| `XO0154` | error | lexer | bad digit, misplaced `_`, missing digits, or out of range integer |
| `XO0155` | error | lexer | unknown duration unit, fractional or non decimal duration |
| `XO0156` | error | lexer | `${` without a matching `}` in the same string |
| `XO0157` | error | lexer | `${}` with no expression |
| `XO0158` | error | lexer | `use` not followed by a valid import path |
| `XO0160` | error | parser | a token that does not fit the grammar here |
| `XO0161` | error | parser | list element followed by a newline instead of a comma (core 1.3) |
| `XO0162` | error | parser | `else` must be on the line of the closing brace (core 1.3) |
| `XO0163` | error | parser | unparenthesized struct literal in a condition (core 1.4) |
| `XO0164` | error | parser | `(`, `[`, or `{` not closed before end of file |
| `XO0165` | error | parser | syntax that is not a pattern (core 3.5) |
| `XO0166` | error | parser | `while cond` used as an expression; only `while { }` is one |
| `XO0167` | error | parser | top level declaration (fn, struct, const, ...) inside a block |
| `XO0168` | error | parser | statement (let, expression, ...) at top level |
| `XO0169` | error | parser | match or select arms separated by `,` instead of newlines |
| `XO0170` | error | parser | several effects in a function type must be parenthesized (core 15) |
| `XO0171` | error | parser | `..` used more than once or not last in pattern fields |
| `XO0172` | error | parser | function declaration without a body |
| `XO0173` | error | parser | `use` after the first declaration (core 15: File) |
| `XO0174` | error | parser | bare `self` parameter outside an interface method signature |
| `XO0175` | error | parser | two statements or arms on one line |
| `XO0176` | error | parser | `pub` before something that cannot be exported |
| `XO0177` | error | parser | syntax from another language (Go, Rust, TypeScript, Python, Swift, Kotlin); names the habit and the Xo form (decision 0087) |
| `XO0178` | error | parser | a line starts with a binary operator (`\|\|`, `&&`, `+`, `==`, ...); end the previous line with it (core 1.3, decision 0136) |
| `XO0179` | error | parser | retired, never reported; was: `var` with a tuple or list pattern; decision 0384 allows `var PATTERN = e` (the code stays reserved) |
| `XO0201` | error | checker | Result value is never used (core 2.6) |
| `XO0202` | error | checker | error type does not convert to the enclosing error type (core 2.6) |
| `XO0203` | error | checker | success of a Result typed T must be written Ok(x) (core 2.6) |
| `XO0204` | error | checker | `?` on an Option in a function returning a Result (core 2.5) |
| `XO0205` | error | checker | `?` where the enclosing function cannot return the failure (core 2.5, 2.6) |
| `XO0206` | error | checker | `let _ =` discards a Result; use `.ignore_err("reason")` (core 2.6) |
| `XO0207` | warning | vet | method chain on the right of `??` starts at a literal, so it applies only to the default (core 15 precedence) |
| `XO0208` | note | vet | `.ignore_err("reason")` discards an error; listed with its reason for review (core 2.6) |
| `XO0209` | error | checker | an expression statement discards the result of a call that only computes it, such as `xs.reverse()` (core 2.6) |
| `XO0301` | error | checker | match does not cover every case (core 2.3) |
| `XO0302` | error | checker | pattern does not fit the type of the matched value (core 3.5) |
| `XO0303` | error | checker | refutable pattern where an irrefutable one is required (core 3.5) |
| `XO0304` | error | checker | alternatives of an or pattern bind different names or types (core 3.5) |
| `XO0305` | warning | checker | `_` arm hides variants of an enum declared in this module (core 3.5) |
| `XO0401` | error | checker | value of the wrong type; only three implicit conversions exist (core 2.10) |
| `XO0402` | error | checker | type arguments cannot be inferred (core 2.7) |
| `XO0403` | error | checker | wrong arguments or fields: count, names, missing, or duplicates |
| `XO0404` | error | checker | type does not implement a required interface or constraint (core 2.4) |
| `XO0405` | error | checker | type cannot be inferred here; annotate it |
| `XO0406` | error | checker | operator, index, slice, or iteration not defined for the type |
| `XO0407` | error | checker | name used as the wrong kind of thing (a type as a value, a value that is not callable) |
| `XO0408` | error | checker | a block const initializer is not a compile time value (core 3.1) |
| `XO0409` | error | checker | invalid explicit conversion (core 2.1) |
| `XO0410` | error | checker | reading a Map by index; use `get` or `get_or_fault` (core 3.6) |
| `XO0411` | error | checker | two fields have one wire key under a derive the struct has (core 2.2, decision 0277) |
| `XO0412` | error | checker | empty wire name, `as ""` (core 2.2) |
| `XO0413` | error | checker | unknown derive argument or `case:` value, or an argument on a derive that takes none (core 2.2) |
| `XO0414` | warning | checker | a wire name on a struct with no derive that maps fields to keys (core 2.2) |
| `XO0415` | error | checker | a top-level constant's initializer performs an effect (core 3.1, decision 0332) |
| `XO0416` | error | checker | a constant's value holds a handle (`Mutex`, `Chan`, `Atomic`, `Task`, a file) (core 3.1, decision 0332) |
| `XO0420` | error | checker | constant std/time layout is invalid: unknown field, bad field form, or stray brace (std.md 13.3) |
| `XO0421` | error | checker | integer literal does not fit the integer type it takes in context (1.6) |
| `XO0422` | error | checker | unknown wire name option; the only one is `omitempty` (core 2.2, decision 0309) |
| `XO0423` | error | checker | a field has two wire strings for one format (`json "a" json "b"`) (core 2.2, decision 0315) |
| `XO0424` | error | checker | a format's wire string (`json`, `sql`, `xml`) on a struct that does not derive that format (core 2.2, decision 0315) |
| `XO0425` | error | checker | a field type, a type parameter list, or a nested struct that `derive Xml` cannot map (core 2.9, decision 0315) |
| `XO0426` | error | checker | a bad `xml` wire string: an unknown or unsupported option, `attr` with a path, `chardata` or `attr` on a type that cannot hold it (core 2.9, decision 0315) |
| `XO0427` | error | checker | `xml.encode_value` or `xml.decode` of a type parameter instead of a concrete struct (core 2.9, decision 0315) |
| `XO0501` | error | checker | var method called on a non var binding (core 3.4) |
| `XO0502` | error | checker | assignment to an immutable binding or to a captured variable (core 5.2, 3.3) |
| `XO0503` | error | checker | one variable or overlapping places passed to two `var` parameters of one call (core 5.2, decision 0111) |
| `XO0510` | error | checker | use of a moved Buf, StrBuilder, BytesBuilder, or std move only handle (core 5.3) |
| `XO0520` | error | checker | mutation of a copied or stored collection element is never written back (core 5.2) |
| `XO0530` | warning | vet | binding the payload of a field shares every heap field of a large value (decision 0263) |
| `XO0531` | warning | vet | a large parameter is only stored while each caller passes its last use, so its heap fields are shared and dropped again (decision 0263) |
| `XO0532` | note | vet | a Map value read into a `var`, changed, and stored back; `Map.update` does it in one lookup (decision 0266) |
| `XO0534` | note | vet | a task captures a value with many heap fields (32 or more), sharing each at the spawn (core 5.1, decision 0294) |
| `XO0535` | note | vet | `${x.debug()}` in a string, which `${x:?}` writes shorter; its fix rewrites it (core 1.6, decision 0296) |
| `XO0536` | note | vet | a collection copied out of a list element, changed, and stored back, so its first change copies all of it; change the element in place (core 5.1, decision 0294) |
| `XO0537` | warning | vet | retired, never reported; was: a by value receiver read before an argument that changes it; decision 0326 defines that order, so the check is gone (the code stays reserved) |
| `XO0538` | warning | vet | a `var` local written after a closure that reads it was created and before the closure is used: the closure keeps the value from its creation (core 5.1) |
| `XO0540` | warning | checker | a `Mutex.with` closure writes its `var` value, then a path returns `Err`, which undoes the write (core 5.4) |
| `XO0601` | error | checker | name does not resolve (core 4.3) |
| `XO0602` | error | checker | import path does not yield a valid local name (core 4.1) |
| `XO0603` | error | checker | name declared twice in one scope, or a predeclared name redefined (core 3.1, 1.4) |
| `XO0604` | error | checker | cyclic module imports (core 4.1) |
| `XO0605` | error | checker | name is not `pub` in its module, or the std type is an opaque handle (core 1.5) |
| `XO0606` | error | checker | method declared outside the module of its type (core 3.4) |
| `XO0607` | error | checker | `break` or `continue` outside a loop, or an unknown label (core 3.5) |
| `XO0608` | error | checker | two C ABI export modules of the native std (std/native/llrt/cabi*.xo) define the same function name, one symbol `xo_x_<name>` (decision 0207) |
| `XO0701` | error | checker | effect performed outside the declared set (core 6.2) |
| `XO0702` | error | checker | unknown effect label (core 6.1) |
| `XO0703` | warning | vet | effect declared in `uses` is never performed (core 6.2) |
| `XO0801` | error | checker | child or group task error does not convert to the enclosing error type (core 7.1) |
| `XO0802` | error | checker | scope handle, or a value containing a task group, escapes its scope (core 7.1) |
| `XO0803` | error | checker | task started without a scope handle (core 7.1) |
| `XO0804` | warning | checker | value read in one `Mutex.with` is written back in a later one (core 5.4) |
| `XO0900` | warning | checker | typed hole report (core 3.7) |
| `XO0901` | warning | vet | `dbg(...)` left in code (core 11.4) |
| `XO0902` | warning | vet | `tls.Verify.None`: a TLS client that checks no certificate (std.md 27, decision 0281) |
| `XO0903` | warning | vet | the error of a filesystem, network, or process call turned into a default (`.ok() ?? d`, or `Err(_) => <empty>` with no NotFound arm), so a transient error gives a smaller answer instead of an error; `// xo:allow XO0903 <reason>` on the line or above it allows one |
| `XO0906` | note | vet | a `match` that only unwraps an Option or a Result or leaves, which `x ?? return v` (`r ?? e => return v`) writes in one line (core 2.5, 2.6, decision 0321) |
| `XO0907` | note | vet --cost | a loop that copies every entry of a map before it starts (a log-mode map through a forked view, or a body the in-place walk does not take; decision 0327) |
| `XO0908` | note | vet --cost | in a loop, an element of a collection made outside it is counted up and down each iteration (passed to a function value whose result has its type, which takes it), or a prelude method hands each element holding heap memory to such a function value (`map`, `filter`, `find`, `any`, `all`, `fold` of a List, Map, or Set; decisions 0327, 0337) |
| `XO0909` | note | vet --cost | a list update in a loop that copies the whole list every iteration: the list is also kept in the same iteration (stored, captured, or a name used after the update), so it is shared when the update runs (decision 0327) |
| `XO0910` | note | vet --cost | a value over 8 words copied where the source shows no copy: captured by a closure, or, in a loop, an element of a collection wrapped whole in an Option or enum value (decision 0327) |
| `XO0911` | note | vet --cost | a view of a parameter's text or bytes (a slice, trim or cut made in a loop, or an element of its split or lines) stored in a field or a returned collection keeps the whole input alive; `clone()` it (decision 0330) |
| `XO0912` | warning | vet | retired in 0.34.0 (no longer reported): `u.0`, the pattern `uuid.Uuid(p)`, or `uuid.Uuid(s)` on a `uuid.Uuid` was deprecated in 0.33.0; a `uuid.Uuid` is now two Ints and those uses are type errors (std.md 24, decision 0387) |
| `XO0913` | note | vet --cost | in a loop, a slice of a Str or Bytes read only by index, length or equality: each iteration makes a view (a range check, a count and a drop); read the whole by offsets (knowledgebase/std-performance-guide.md) |
| `XO0914` | note | vet --cost | in a loop, a List of at most 8 elements made and only filled and read in the iteration: an allocation and a free every iteration; keep the values in locals or reuse one list made before the loop (knowledgebase/std-performance-guide.md) |
| `XO0904` | warning | vet | a file an embed constant embeds is ignored or untracked by git, so a build of a clean checkout embeds something else or fails (decision 0304) |
| `XO1001` | error | reserved | `std/sys/<os>` used outside the matching target arm (core 12.2) |
| `XO1101` | error | checker | Go package cannot be bound (not found, does not build, main package, or binding error) |
| `XO1102` | error | checker | Go name exists but its signature does not map to Xo, so the binding skipped it |
| `XO1103` | note | vet | per module report of `use go` packages and the functions that perform `go` (decision 0041) |
| `XO1104` | error | checker | C header cannot be bound (not found, clang cannot read it, a `link` file is missing, or an unknown effect in its `uses`) |
| `XO1105` | error | checker | C function exists in the header but its signature does not map to Xo, so the binding skipped it |
| `XO1106` | error | checker | a value with no Go value passed to Go as `any`: `go.any(x)`'s argument or an element of a list literal for a `...any` parameter (decision 0412) |
| `XO1201` | error | no GC check, checker (`use c` without `--no-gc`) | construct, std API, or effect not available on the native backends (arm64, amd64, LLVM); planned or impossible |
| `XO1301` | error | modules | malformed xo.mod require or deny line, xo.lock, or xo.work line, or a deny rule naming a package that does not exist (core 4.2, 4.3) |
| `XO1302` | error | modules | module version cannot be fetched from git or a module proxy (XO_PROXY): repository, tag, or version missing, or its xo.mod declares another path |
| `XO1303` | error | modules | module version is not in the module cache and the build is offline (XO_OFFLINE, --offline) |
| `XO1304` | error | modules | module tree does not match its xo.lock content hash (core 4.3) |
| `XO1305` | error | modules | xo.lock has no hash for a module version the build uses; run `xo mod tidy` |
| `XO1306` | error | checker | import of a package from a module that xo.mod does not require |
| `XO1307` | error | modules | vendor/ does not match xo.mod; run `xo vendor` |
| `XO1308` | error | build | release build (`xo build --release`, or `--ci`) while an xo.work workspace overrides required modules |
| `XO1309` | error | checker | call into a dependency performs an effect above its ceiling (`require ... uses ...` in xo.mod, `use ... uses ...` in xo.work) |
| `XO1310` | error | publish | `xo publish`: a minor or patch release removes or changes an export, or adds an effect to one |
| `XO1311` | error | publish | `xo publish`: the release cannot be made (not in a git repository, a subdirectory module path not ending in its directory, uncommitted changes, tag exists, version not above the last, wrong /vN path) |
| `XO1312` | error | checker | `use` of a package that a `deny` rule of xo.mod forbids (decision 0095) |
| `XO1313` | error | modules | a fetched module version disagrees with the checksum log (XO_SUMDB), or the log's signature or proofs do not verify (4.3, decision 0124) |
| `XO1314` | error | modules | the checksum log cannot be consulted, or XO_PROXY, XO_SUMDB, XO_NOPROXY, or XO_NOSUMDB is malformed (4.2, decisions 0124, 0126) |
| `XO1315` | note | checker | `edition 2026` in xo.mod (or `xo 2026` in xo.work) is the old name of `edition preview`; the fix rewrites it (13, decision 0155) |
| `XO1316` | error | checker, xo get | `use c` or `use go` of an impure Go package in a required module without the consumer's `vouch` for that version (4.2, decision 0156) |
| `XO1317` | error | modules | a dependency's effects are wider than its `effects` line in xo.lock (4.3, decision 0156) |
| `XO1318` | error | modules | a module's tree cannot be read (an I/O error) for its content hash, the effect or escape hatch audit, or `xo mod tidy`'s import scan, so none of them answers from a partial tree (4.3, decision 0156) |
| `XO1319` | error | modules | a `use go` of a package outside Go's standard library whose Go module has no `require go` line in xo.mod (builds would follow its latest version); the fix adds the line with the version the binding resolved, and `xo mod tidy` adds it too (decision 0356) |
| `XO1320` | error | modules | `xo mod tidy` with a `require go` pin whose go.sum hashes no binding has fetched yet (a fresh clone: .xo/gobind is not committed) and xo.lock does not hold: nothing is written; run `xo check` once, then tidy again (decision 0356) |
| `XO1401` | error | checker | allocation in a real-time function (literal, interpolation, `+` on Str or List, closure, growth method) |
| `XO1402` | error | checker | loop without a provable bound in a real-time function, or a `bound N` that is not a constant or is exceeded by a constant range |
| `XO1403` | error | checker | call in a real-time function of a function that is not real-time, a function value, or an interface method |
| `XO1404` | error | checker | recursion among real-time functions |
| `XO1405` | error | checker | blocking operation (channel, Mutex, scope, select, task) or capability effect in a real-time function |
| `XO1501` | error | build | `--stamp NAME=VALUE` names no `const NAME: Str = "..."` (a plain string literal) of the main module (decision 0165) |
| `XO1502` | error | build | the go command the Go backend runs (`$XO_GO`, else `go` on PATH) is older than Go 1.24, the oldest Go that builds generated programs and the runtime (decision 0176) |
| `XO1503` | error | build | the Go backend finds no go command: `$XO_GO` is not an executable file, or there is no `go` on PATH; the native backends need none (decision 0176) |
| `XO1504` | error | build | the program uses a std API the native runtime does not support on the build's target yet: reported by no build since stage 2 of decision 0197 (2026-10-12; stage 1 left out processes on Windows), and Windows builds default to the amd64 backend (decisions 0254, 0270); the message would name them, and the Go backend builds the program |
| `XO1505` | error | build | the LLVM backend's clang (`$XO_CLANG`, else `clang`, or `--cc`) is an LLVM clang older than 16, whose LLVM lacks llvm.threadlocal.address; Apple's clang (numbered apart) is not checked (gotchas/llvm-thread-local-constant.md) |
| `XO1506` | error | checker | `embed("...")` names no file, more than one for a `Str` or `Bytes` constant, a file that cannot be read, or more than an embed may hold (64 MiB each, 256 MiB a program; decision 0304) |
| `XO1507` | error | checker | an `embed` path leaves the module: absolute, `..`, `.`, an empty segment, or a symbolic link (decision 0304) |
| `XO1508` | error | checker | a `Str` embed constant's file is not valid UTF-8; declare it `Bytes` (decision 0304) |
| `XO1509` | error | checker | `embed` used other than as the initializer of a constant typed `Str`, `Bytes`, or `embed.Dir`, with one plain string literal (decision 0304) |
| `XO1510` | error | build | a function the native backends are about to generate disagrees with this build's definitions: a call's argument or result types, a closure, or a field or payload access do not match the callee or type (decision 0342: a stale cached piece, such as a front end body naming a helper this build defines otherwise); rebuild with `XO_BACKEND_CACHE=off` and report it |
| `XO1511` | warning | checker | `read`, `read_text`, or `contains` on an `embed.Dir` constant, or a literal `.sub` of one, with a literal path it holds no file at; a key ending in the path is offered as a fix (decision 0304) |
| `XO1512` | error | build | under `XO_BACKEND_CACHE=verify`, a build, run, or test the run cache would have served differs from its rebuild, in the binary or in what the build printed: an input the entry does not list, or a build that is not reproducible (decision 0360) |
| `XO1513` | error | build | a command the WASI build of xo (`xo` built for wasip1/wasm) does not offer: it needs a process, a socket or a system tool that WASI preview1 does not have (`run`, `test`, `serve`, `get`, `build --backend=llvm`, ...); printed as `xo: COMMAND is not available in the WASI build`, exit status 2 (decision 0405) |

`XO1001` is reserved: no `std/sys/<os>` module exists yet (14), so it is
never reported.

Faults (8) that have a stable code print it in their message:

| Code | Fault |
|---|---|
| `XO-F001` | deadlock: every task is blocked on another task (11.4) |
| `XO-F004` | `Mutex.with` or `with_read` called re entrantly on a mutex the task holds (5.4) |
| `XO-F005` | a shielded `defer` ran longer than 5 seconds (3.5) |
| `XO-F006` | a loop started more iterations than its `bound N` (3.5) |
| `XO-F007` | a real-time section allocated (6.5, native builds) |
| `XO-F008` | shielded code under a resource budget ran past twice a ceiling (std.md 7, decision 0173) |
| `XO-F009` | a constant read during its own initialization, through a function its initializer calls (core 3.1, decision 0332) |

---

## 11. Debugging [P1 basics, P2 replay]

### 11.1 Error traces

Each `?` that propagates an error appends a frame:

```
LoadErr.Db(msg: "timeout")
  at users.fetch_user(id=42)          users/fetch.xo:18
  at api.get_profile(req=GET /u/42)   api/profile.xo:7
```

- Frames record function, location, and parameters that implement `Debug`,
  truncated to 64 bytes each. `Secret[T]` prints `<redacted>`. Capability
  parameters are left out. Frames are built only when an error
  propagates; the success path records nothing.
- Frames are recorded in every build, debug and release (decision 0092).
  A frame keeps a copy of each parameter value and produces its `Debug`
  text only when the trace is printed (the error report of `main`, a
  crash bundle, a test failure, a handler's error log), formatting no
  more of a value than the 64 bytes it shows. The text is what it would
  have been at the `?`: values are immutable or copied on write, and the
  few whose `Debug` text can change in place (`Buf`, `Atomic`) are
  formatted when captured.
- `--error-args=off` (any build, decision 0354; `--error-args=false`
  too) records the function and location only and prints `(...)` for
  the parameters: `at users.fetch_user(...)   users/fetch.xo:18`.
- **[later]** A last line naming the task and its scope
  (`in task "handler#913" (scope api.serve)`) is planned.
- Native backends (`--no-gc`, LLVM, arm64) record the same frames
  (decision 0104) and print them in main's error report; test reports,
  handler logs, and crash bundles do not show them there yet (10.1).

### 11.2 Crash bundles

On a fault reaching `main`, an unhandled error from `main`, or an isolated
fault in a server, the runtime writes `<name>-<time>.xocrash` containing: the
error or fault with frames, the task tree, build id, source hashes, target, and
**[P2]** the effect trace for the failing request, and prints its path on
standard error (`crash bundle: PATH`). Bundles go to `$XO_CRASH_DIR`, else
`<user cache>/xo/crash`, which keeps the newest 100 (owner, 2026-10-22)
and which `xo clean -cache` does not empty. Native builds (`--no-gc`)
write none yet (decision 0085).

### 11.3 Record and replay [P2]

All effect calls go through capability values, so the runtime can log each
call and its result, plus scheduler decisions.

- `xo run --record=trace.xotrace` (or `XO_RECORD=trace.xotrace` for any built
  binary) records a full run. Values of type `Secret[T]` are recorded redacted;
  `xo replay` then requires them from the environment and exits with status 3
  naming the missing one.
- Calls into Go packages (`use go`, 4.4) are recorded at the boundary
  (decision 0093). A call whose arguments and results are all boundary
  values (scalars, `Str`, `Bytes`, `Time`, `Duration`, lists and maps of
  those, `go.Error` with its text and Go type) and that takes no callback
  is an effect `go.call(<name>, <argument hash>)` with its results; `xo
  replay` checks the name and the hash (a different argument diverges)
  and returns the recorded results without calling Go. A Go panic is
  recorded and raised again as the same fault. A call whose result, an
  argument, or the receiver is a Go handle, or that takes an Xo callback,
  runs again in the replay: a handle's Go state exists only if the calls
  that make and change it run (`sync.WaitGroup.wait` must wait for real).
  The trace marks such a call (`"x": "handle"` or `"callback"`), the
  replay checks that it comes at the same point, and `xo replay` reports
  it once per name: ``re executed foreign call `os.open` ``. Functions
  marked `replay rerun` (4.4) run again and are not recorded. A trace
  written before decision 0093 (no `"foreign"` field in its header) runs
  every Go call again, as before.
- Calls into C (`use c`, 4.5) follow the same rule once native builds
  record (10.1: they do not yet): a call whose arguments and results are
  boundary values is answered from the trace as `ffi.call(<name>,
  <argument hash>)`, and one that passes or returns a C handle runs
  again. Each binding's doc comment already says which applies
  (`/// Replay: by value` or `/// Replay: a C handle crosses`).
- An Xo callback that Go calls on a goroutine of its own is recorded
  (where the scheduler admitted it, with its arguments) and replayed at
  the same point, and a replay diverges when Go calls it with other
  arguments (decision 0073).
- In production, a per request ring buffer (`XO_RING=N` requests) is kept and
  attached to crash bundles.
- `xo replay trace.xotrace` (or a `.xocrash`) re executes the program offline
  with identical effect results and interleavings.
- `xo debug --replay trace.xotrace` supports stepping backward.
- `xo why <expr> --at file:line` traces where a value came from (the
  assignments, calls, and effect results that produced it) using the replay.

### 11.4 Live inspection

- `xo dump <pid>` (or SIGQUIT) prints the task tree: each scope, its children,
  and what each task is blocked on, with locations. The program keeps running
  (unlike a Go program's default SIGQUIT, which exits); the dump goes to the
  program's standard error and to `$XO_DUMP_DIR` (or the temp dir) as
  `xo-dump-<pid>.txt`, a new file of mode 0600 that replaces an earlier one
  and is never written through a symbolic link. On the native backends
  (decision 0251) a task's wait is named as on the Go backend (`blocked in
  receive at main.xo:5`); the scopes' own sites and the holders of a mutex
  are not shown yet. A test binary has no dump.
- **[P2]** Deadlocks are detected by the runtime and reported as a wait cycle
  with locations (fault `XO-F001`, exit status 70, crash bundle). A task
  counts as blocked when it waits on another task (channel, mutex, semaphore,
  `await`, scope end, `Once`); waits on timers, `after`, `task.stopping()`,
  signals, and I/O do not. In test worlds the report is immediate; real runs
  report after 200 ms with no progress. A task inside a Go call (`use go`)
  counts as blocked only while it waits for its own Go callbacks running on
  goroutines of Go's, those callbacks are blocked in turn, and every
  goroutine's stack shows that nothing else can move (the Go call parked in
  a `sync` wait, no goroutine running, sleeping, or in I/O); the report
  shows the edge as `blocked in Go call sync.(*WaitGroup).Wait at ... for
  callback 3`. A Go call waiting on a Go channel, select, or timer is never
  reported, and nothing is reported while any Go runtime timer is pending
  (a `time.AfterFunc` may release the wait; the check looks again after it
  fires); builds for targets other than darwin and linux never report it
  (decision 0073). Native builds (decision 0068), which
  have no timers, I/O, or signals, report at once when every task is blocked
  and none is queued, with the fault's first line only (no location or list
  of blocked tasks yet).
- `dbg(expr)` prints the expression source and its `Debug` value, returns the
  value, and is rejected by `xo build --ci` (decision 0265). It prints one
  line on standard error, `[file.xo:LINE:COL] expr = value`, on every
  backend, `--no-gc` included. It needs no capability: it is the one output
  a function may produce without `Stdio`, and it is not part of a
  function's effects, so adding one changes no signature. `xo build
  --release` and `--dbg=off` (any build, decision 0354) compile
  `dbg(expr)` to `expr` (the value is computed, nothing prints); other
  builds keep it, and `--release --dbg=on` keeps it in a release build. `xo vet` warns (XO0901), and
  real-time functions reject it (XO1405) because its write blocks.

### 11.5 Debugger protocol

`xo debug --json` speaks a JSON line protocol (breakpoints, conditional
breakpoints, break on contract failure, eval, step, step back with replay).
`xo debug --dap` speaks the Debug Adapter Protocol for editors.

The protocol's version 1 (decision 0253; compiler/debug session.xo; until
the process control of stage 2 it runs on a recorded thread as `xo dev
debug --json`): each command (decision 0253 point 4) gives one line, an
object with `v` (1), `cmd` (the command's long name: `break`, `delete`,
`run`, `continue`, `step`, `next`, `finish`, `print`, `locals`,
`backtrace`, `frame`, `tasks`, `task`, `threads`, `quit`; `""` for a line
that is not a command), and `ok`. With `ok` false the object has `error`
(the text), nothing else. With `ok` true, by command:

- `break`: `id`, `locations` (each `{pc, file, line}`; `pc` is a `"0x..."`
  string, `file` a base name).
- `delete`: `id`.
- `backtrace`: `frames`, each `{index, pc, function, inlined, file, line,
  selected}`; an inlined call is a frame of its own (`inlined` true) with
  its physical frame's `pc`, innermost first.
- `frame`: `frame`, one frame as in `backtrace`.
- `locals`: `frame` (the selected frame's index), `vars`, each `{name,
  type, param, text}` and `value`, or `error` instead of `value` when the
  variable cannot be read there (`text` then says why, as
  `<optimized out>`).
- `print`: `frame`, `path` (`p.a.x`), `text`, `value`.
- `threads`: `threads`, each `{id, selected}` (and `recorded` for a
  recorded thread).
- `quit`: no more fields.

A `value` is `{"text": T}` for a scalar (`T` its text: an integer keeps
its 64 bits, a float has Xo's float text, `true` or `false`, a pointer
`0x...`), `{"type": NAME, "fields": [{"name", "value"}, ...]}` for a
structure, and `{"type": "[N]", "elements": [value, ...]}` for an array.
Fields are added to a version without changing `v`; removing or changing
one takes the next `v`, as an `XOnnnn` code keeps its meaning.

**[P1]** The generated Go code carries `//line file.xo:N` directives, so Delve,
Go stack traces, and pprof report Xo source locations.

### 11.6 Observability [P1, export later]

What exists (decision 0325): `std/metrics` (std.md 37) has counters,
gauges and histograms with label values in a registry that main makes and
passes on, their Prometheus text exposition (`Registry.text`, the
`GET /metrics` handler `Registry.handler`), and an HTTP middleware that
counts and times every request. `std/trace` (std.md 38) gives each request
an id and keeps a bounded ring of spans (a request's span and a program's
own), which a handler reads; it has no exporter.

Not implemented yet [later]: spans are a program's own (a `scope` or an
effect call is not a span by itself), log records do not carry the
current span or request id automatically, and there is no OpenTelemetry
(OTLP) export: `XO_OTEL_ENDPOINT` is not read. A `traceparent`-style
exporter is a program's own code over `Ring.spans`.

---

## 12. Targets and platform code [P1]

### 12.1 Targets

`xo build --targets=all` builds every supported target from any host:
`linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 wasi/wasm32`.
With no `--backend`, each target builds with its own native backend
(arm64, amd64, or wasm for wasi), and `all` is the six targets a native
backend builds: windows/arm64 needs `--backend=go`, whose `all` is all
seven, and a note says so (decision 0270 addendum, owner 2026-10-22).
Naming windows/arm64 in a list with no `--backend` is an error, reported
after the other targets are built (exit status 1). Several targets write
`NAME-OS-ARCH` binaries (`.exe` for windows, `.wasm` for wasi) into `-o`'s
directory; `-o NAME` for a windows target with no extension writes
`NAME.exe`.

Binaries are static and builds are reproducible: the same source in the
same directory, lockfile, and toolchain version produce byte identical
output (the binary records the absolute source directory, which crash
bundles and `xo replay` use, so a copy elsewhere differs in those bytes).
Each build writes an SBOM (`<bin>.sbom.json`). **[later]** No SBOM is
written yet.

### 12.2 Platform specific code

There are no build tags and no filename based platform selection.
`target` is a predeclared compile time constant with fields
`os: TargetOs` (`Linux Darwin Windows Wasi`) and `arch: TargetArch`
(`Amd64 Arm64 Wasm32`):

```
pub fn config_dir(env: Env) -> Str ! Error uses env {
  match target.os {
    Linux => env.get("XDG_CONFIG_HOME") ?? "${env.home()?}/.config"
    Darwin => "${env.home()?}/Library/Application Support"
    Windows => env.get("APPDATA").or_err("APPDATA not set")?
    Wasi => "/config"
  }
}
```

- A `match` on `target.os` or `target.arch` is resolved at compile time; only
  the chosen arm is code generated.
- **Every arm is type checked for every target on every `xo check` and
  `xo build`**, so a broken Windows branch fails on a macOS machine. A missing
  OS is a non exhaustive match error.
- Platform specific APIs live in `std/sys/<os>` and can only be referenced from
  inside the matching arm (error `XO1001` otherwise). **[later]** No
  `std/sys/<os>` module exists yet, so `XO1001` is reserved (10.6).
- The native backends build `target` as a constant of the build's target,
  so only the chosen arm is code generated (decision 0107).

---

## 13. Editions and compatibility [P1]

- `xo.mod` declares an `edition`. The compiler supports every edition ever
  released. Modules on different editions link together.
- Before 1.0 the only edition is `preview` (decision 0155). It is not
  frozen: during the 0.x releases a minor release may change it (decision
  0089), and `xo fix` carries the migration. `edition 2026`, its name
  before decision 0155, is an alias of `preview` during the 0.x releases;
  `xo check` reports the fixable note `XO1315` on it and `xo fix`
  rewrites it.
- At 1.0 the first frozen edition is named and `xo fix` rewrites
  `edition preview` to it mechanically. The compatibility promise below
  starts with that edition, not with `preview`.
- A new edition may change syntax and defaults, never the meaning of code in
  older editions.
- `xo fix --edition=<name>` migrates a module mechanically and must leave it
  compiling. An edition is not released until its rewriter is complete.
- The standard library follows the same rule: `std` APIs are never removed
  within an edition; deprecated APIs ship rewrites in `xo fix`.

---

## 14. Standard library

The prelude (methods on builtin types) and the standard library surface are
specified in `spec/std.md`. Modules:

| Module | Contents |
|---|---|
| `std/http` | Router, server, client, requests, responses, middleware, error to status mapping |
| `std/json` | Derive based encode and decode, `Patch[T]` for partial updates |
| `std/log` | Structured logging; performs the `log` effect (6.1) |
| `std/task` | `isolate`, `with_timeout`, `is_canceled`, `Semaphore` |
| `std/time` | Time zones, civil dates and times, layouts with named fields (arithmetic is in the prelude; std.md 13) |
| `std/testing` | Fake capability types, generators, captured logs |
| `std/sql` | Database handles over drivers, in memory driver (draft) |
| `std/crypto` | SHA-256, HMAC, constant time compare, random tokens, password hashing |
| `std/path` | Pure helpers for `/` separated paths |
| `std/cli` | Command line parsing: flags, arguments, subcommands, help, usage errors (std.md 14) |
| `std/term` | Terminal colors that follow NO_COLOR and FORCE_COLOR (std.md 15) |
| `std/csv` | CSV and TSV parsing and formatting (std.md 16) |
| `std/regexp` | RE2 regular expressions (std.md 17), on every backend (natively in Xo, decision 0172) |
| `std/archive` | gzip, tar, and zip in memory (std.md 18), on every backend (natively in Xo, decision 0172; zip64 archives are an error natively) |
| `std/toml` | TOML 1.0 documents decoded with the JSON rules (std.md 19) |
| `std/go` | `go.Error`, the error of a Go call (4.4) |
| `std/trace`, `std/config`, `std/text`, `std/sys/<os>` | Later drafts |

---

## 15. Grammar summary (informal EBNF)

Every comma separated list allows a trailing comma and may span lines. The
parser of `xo` (`compiler/parser`; `internal/parser` until decision 0215)
is the reference implementation of this grammar.

```
File        = { UseDecl } { TopDecl } .
UseDecl     = "use" ImportPath [ "as" ident ]
            | "use" "go" string [ "as" ident ]                      // 4.4
            | "use" "c" string { "link" string } [ "as" ident ]
              [ "uses" ( "none" | Effect { "," Effect } ) ] .       // 4.5
ImportPath  = PathSeg { "/" PathSeg } .               // std/http, github.com/acme/x
PathSeg     = PathChar { PathChar } .                 // xo-lang, rate-limit, v2, example.com
PathChar    = letter | digit | "_" | "-" | "." .      // ASCII only; no empty, `.`, or `..` segment (XO0158)
TopDecl     = { DocComment } [ "pub" ] ( FnDecl | StructDecl | EnumDecl | IfaceDecl
            | TypeDecl | ConstDecl ) | TestDecl .

FnDecl      = "fn" [ Receiver ] ident [ TypeParams ] Params [ "->" Type ] [ ErrSpec ]
              { FnClause } Block .
FnSig       = "fn" ident [ TypeParams ] Params [ "->" Type ] [ ErrSpec ] { FnClause } .
Receiver    = "(" [ "var" ] ident ":" Type ")" .
TypeParams  = "[" TypeParam { "," TypeParam } "]" .
TypeParam   = ident [ ":" ( Type | "Effects" ) ] .
Params      = "(" [ Param { "," Param } ] ")" .
Param       = [ "var" ] ( ident ":" Type [ "=" Expr ] | "self" ) .   // self: interface methods only
ErrSpec     = "!" [ Type ] .
FnClause    = "uses" ( EffectList | "(" EffectList ")" )
            | "requires" [ "(" "debug" ")" ] Expr [ "else" Expr ]
            | "ensures" [ "(" "debug" ")" ] Expr .
EffectList  = Effect { "," Effect } .
Effect      = ident [ "." ident ] .

Type        = BaseType { "?" } .
BaseType    = QualName [ "[" TypeArg { "," TypeArg } "]" ]
            | "(" ")" | "(" Type ")" | "(" Type "," [ Type { "," Type } ] ")"
            | FnType .
TypeArg     = Type | EffectList .                     // effect sets for fx parameters
FnType      = "fn" "(" [ FnTParam { "," FnTParam } ] ")" [ "->" Type ] [ ErrSpec ]
              [ "uses" ( Effect | "(" EffectList ")" ) ] .
FnTParam    = [ "var" ] Type .

StructDecl  = "struct" ident [ TypeParams ] "{" { Field } "}" [ Invariant ] [ Derive ] .
Field       = { DocComment } [ "pub" ] ident [ "as" ( string | rawstring ) ] ":" Type [ "=" Expr ] .
Invariant   = "invariant" Expr .
Derive      = "derive" DeriveItem { "," DeriveItem } .
DeriveItem  = QualName [ "(" ident ":" ident { "," ident ":" ident } ")" ] .
EnumDecl    = "enum" ident [ TypeParams ] "{" { Variant } "}" [ Derive ] .
Variant     = { DocComment } ident [ "(" VField { "," VField } ")" ] .
VField      = ident ":" Type | "from" Type .
IfaceDecl   = "interface" ident [ TypeParams ] "{" { FnSig } "}" .
TypeDecl    = "type" ident [ TypeParams ] ( "=" Type | "struct" "(" Type ")" [ Derive ] ) .
ConstDecl   = "const" ident [ ":" Type ] "=" Expr .
TestDecl    = "test" string [ "for" Param { "," Param } ] [ "seed" int ]
              [ "uses" EffectList ] Block .

Stmt        = "let" Pattern [ ":" Type ] "=" Expr
            | "var" ident [ ":" Type ] "=" Expr
            | ConstDecl
            | Place AssignOp Expr
            | "defer" ( Stmt | Block ) | "return" [ Expr ]
            | "break" [ ident ] | "continue" [ ident ]
            | ForStmt | WhileStmt | Expr .
AssignOp    = "=" | "+=" | "-=" | "*=" | "/=" | "%=" .
Place       = ident { "." ident | "." int | "[" Expr "]" } .
ForStmt     = [ ident ":" ] "for" Pattern "in" Expr [ "bound" Expr ] Block .
WhileStmt   = [ ident ":" ] "while" [ ( Expr | "let" Pattern "=" Expr ) [ "bound" Expr ] ] Block .

Expr        = IfExpr | MatchExpr | ScopeExpr | SelectExpr | Block | FnLit | Binary
            | "while" Block | "???" .      // infinite loop as an expression, type Never
Binary      = Operand { BinOp Operand } [ "??" ( Exit | ErrArm ) ] .
ErrArm      = ident "=>" ( Expr | Stmt ) .   // r ?? e => body (decision 0321)
Exit        = "return" [ Expr ] | "break" [ ident ] | "continue" [ ident ] .   // type Never
Operand     = Unary [ "is" Pattern ] .            // `is` binds like `==`
FnLit       = "fn" "(" [ LParam { "," LParam } ] ")" [ "->" Type ] [ ErrSpec ]
              [ "uses" ( EffectList | "(" EffectList ")" ) ] Block .
LParam      = [ "var" ] ident [ ":" Type ] .
IfExpr      = "if" ( Expr | "let" Pattern "=" Expr ) Block [ "else" ( IfExpr | Block ) ] .
MatchExpr   = "match" Expr "{" { Pattern [ "if" Expr ] "=>" ( Expr | Stmt ) } "}" .
ScopeExpr   = "scope" ident Block .
SelectExpr  = "select" "{" { SelectArm } "}" .
SelectArm   = ( [ ident "=" ] Expr | "after" Expr ) "=>" ( Expr | Stmt ) .
Postfix     = Primary { "." ident | "." int | Call | Index | "?" | "with" StructLit } .
Index       = "[" ( Expr | [ Expr ] ( ".." | "..=" ) [ Expr ] ) "]" .
Call        = [ "[" TypeArg { "," TypeArg } "]" ] "(" [ Arg { "," Arg } ] ")" .
Arg         = [ ident ":" ] Expr .
Literal     = int | float | duration | string | rawstring | bytes | rune | "true" | "false" .

Pattern     = PatAlt { "|" PatAlt } .
PatAlt      = "_" | ident | [ "-" ] Literal [ ( ".." | "..=" ) [ "-" ] Literal ]
            | "(" ")" | "(" Pattern ")" | "(" Pattern "," [ Pattern { "," Pattern } ] ")"
            | [ QualName "." ] ident [ "(" ( ".." | PatFields ) ")" ]   // variant
            | QualName "{" PatFields "}"
            | "[" [ PatElem { "," PatElem } ] "]" .               // ".." at most once
PatElem     = Pattern | ".." .
PatFields   = PatField { "," PatField } [ "," ".." ] .
PatField    = [ ident ":" ] Pattern .
QualName    = [ ident "." ] TypeName .     // optional module prefix
TypeName    = ident .
```

Operator precedence, highest first: postfix (`. () [] ?`), unary (`- ! ~`),
`* / % << >> &`, `+ - | ^`, `.. ..=`, `== != < <= > >= is`, `&&`, `||`, `??`.

Parsing rules the grammar alone does not settle:

- **Type arguments vs. indexing.** `x[...]` holds type arguments when every
  element looks like a type (`UpperCamel`, `mod.UpperCamel`, `(`, or `fn`) and
  either `(` follows or `x` is an `UpperCamel` type name; otherwise it is an
  index. So `json.decode[User](b)` and `Mutex[Int].new(0)` take type
  arguments, while `xs[i]` and `m[Red]` index. An effect argument (a
  lowercase `name` or `name.sub`) also counts as a type element, except that
  a base spelled like a constant (`UPPER_SNAKE`) with only lowercase names
  in the brackets is an index: `NAMES[i]` (decision 0143).
- **`{` in expression position** is an empty map when it is `{}`, a map literal
  when a `:` appears at depth zero before the first newline, and a block
  otherwise. A match arm `=> {}` is therefore an empty map; write `()` for unit.
- **Function types in return position** take a trailing `uses` themselves:
  `-> fn(A) -> B uses clock {` gives the effect to the returned function. Write
  `-> (fn(A) -> B) uses clock {` to put it on the declaration.
- In a function type, a `uses` list with more than one effect must be
  parenthesized, because a following comma would otherwise be ambiguous.
- Only `while { }` (no condition) may appear in expression position.
- Triple quoted strings support escapes and interpolation; a blank first and
  last line are dropped; the common indentation is measured on the source.
- An interpolation is `"${" Expr [ ":?" ] "}"` (1.6): `:?` asks for the
  Debug text (decision 0296).

---

## Appendix A: complete example

```
// api/main.xo
use std/http
use std/log

pub struct User {
  pub id: Int
  pub name: Str
  pub email: Str?
} derive Json

pub enum ApiErr {
  BadRequest(msg: Str)
  NotFound(id: Int)
}

// http maps handler errors to responses through this interface.
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

struct Store {
  users: Mutex[Map[Int, User]]
}

fn (s: Store) get(id: Int) -> User ! ApiErr
  requires id > 0 else ApiErr.BadRequest(msg: "id must be positive")
{
  s.users.with(fn(m) { m.get(id) }).or_err(ApiErr.NotFound(id: id))?
}

fn routes(store: Store) -> http.Router {
  var r = http.router()
  r.handle("GET /users/{id:Int}", fn(req) -> http.Response ! ApiErr {
    let u = store.get(req.param_int("id"))?
    http.json(200, u)
  })
  r
}

fn main(os: Os) ! {
  let store = Store{users: Mutex.new({1: User{id: 1, name: "Ada", email: None}})}
  let srv = http.server(os.net.listen(":8080")?, routes(store))
  log.info("listening", {"addr": ":8080"})
  srv.serve()?
}

test "missing user is 404" {
  let store = Store{users: Mutex.new({})}
  let resp = routes(store).call(http.new_request("GET", "/users/9"))
  expect(resp.status) == 404
}
```

## Appendix B: open questions

Tracked in `knowledgebase/decisions/` once decided.

1. ~~Method syntax: Go style receivers or `impl` blocks.~~ Decided in
   0090: receivers.
2. ~~`Map` iteration order.~~ Decided in 0091: insertion order.
3. ~~Error frame capture cost in release builds.~~ Decided in 0092:
   frames on in every build, captured lazily (implemented on the Go
   backend; the native backends record them too, decision 0104).
4. ~~How `http` handler errors map to status codes.~~ Decided in 0019.
5. ~~FFI design and record/replay.~~ Decided in 0093: `use go` and
   native `use c`, foreign calls recorded at the boundary (`use go`
   recording implemented; `use c` is implemented (4.5, decision 0116)
   without recording, since native builds do not record yet).
6. ~~Interpolation syntax.~~ Decided in 0021: `${x}`.
