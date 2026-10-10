# Changelog

Xo stays on 0.x until the owner declares 1.0 (decision 0089). The toolchain
minor matches the spec draft; the full list of language changes for each
release is the "Changes in 0.N" section of `spec/core.md`.

## Unreleased

- The self hosted checker checks generics, effects, and real-time
  functions (`selfhost/check`, stages 3 and 4 of decision 0141): it
  prints the sigs section of `xo typedump` (type parameters, constraints,
  implicit parameters, declared and inferred effects) and reports XO0402,
  comparability (XO0404), XO0701, and XO1401 to XO1405 as internal/check
  does, on every repository module (go, arm64, and LLVM backends). Effect
  ceilings of dependencies (XO1309) are not ported yet.
- The self hosted checker types every expression (`selfhost/check`,
  stage 2 of decision 0141): inference, literals, operators, expected
  types and implicit conversions, calls with named arguments, fields,
  methods, statics, variants, closures, and the Option and Result rules,
  with the same types as internal/check on every repository module it
  accepts (go, arm64, and LLVM backends) and the same diagnostics on
  modules rejected only for names and types. The names section of `xo
  typedump` now also names the fields, methods, statics, variants, and
  parameters (of named arguments) that identifiers resolve to.
- A call reads a `var` receiver or `var` argument after computing its
  arguments (spec 5.2, decision 0142). On the native backends
  `t.get(t.add(7))` ran `get` without the item `add` pushed, and
  `xs.push(f())` dropped what `f` pushed onto `xs`.
- `NAMES[i]`, a constant indexed by a lowercase name, is an index
  (decision 0143); it parsed as type arguments with an effect argument
  and was XO0407.
- The type checker's name resolution written in Xo (`selfhost/check`,
  decision 0141), the first stage of the self hosted checker: the same
  resolution of every identifier as internal/check on every repository
  module the Go checker accepts (on the go, arm64, and LLVM backends),
  and the same diagnostics on modules rejected only for names. Hidden
  `xo typedump [--section=names,types,sigs,diags] [--each] <dirs>`
  prints what the Go checker resolved, a section at a time
  (bench/results/selfhost-checker-2026-10-10.md).
- An assignment computes its value before it reads the binding it
  stores into (spec 5.2, decision 0140). On the native backends
  `t.objs[o].kids = t.add_kids()` dropped what `add_kids` pushed onto
  `t.objs`.
- Native backends: two modules may declare types of the same name; they
  were one type (a field of one was "not available without a garbage
  collector", or the wrong layout was used).
- Go backend: `??` on a nested optional (`opt.ok() ?? None` where `opt`
  is a `T? ! E`) generated Go that did not compile.
- `XO0401` on an `if` without `else` where a value is expected now has
  a note naming the missing `else`; when the expected type came from a
  sibling branch (a closure ending with `if a { n.add(1) } else if b
  { ... }`), a fix discards the branch values with `let _ =`.
- New error `XO0178` (decision 0136): a line that starts with a binary
  operator (`|| b`, `&& c`, `+ d` continuing the line above) gets one
  diagnostic and a fix that moves the operator to the end of the
  previous line. Before, a leading `||` was taken for a Rust closure
  (a wrong XO0177 fix) and cascaded into XO0160 and XO0175.
- `for b in data` over `Bytes` (spec 3.5, decision 0138) yields each
  byte as a `UInt8`; it was `XO0406`. Go's `for _, b := range data`
  gets the fix `for b in data`.
- `while let` (spec 3.5, decision 0137): `while let Some(v) = q.pop() { }`
  loops while a refutable pattern matches; `while let v = opt` is the
  Option shorthand, as for `if let`. Labels, `continue`, and `bound N`
  work; every backend and the self hosted parser.
- Import paths with a `.` or `..` segment (`use example.com/app/./lib`,
  `use ./lib`) are `XO0158`. They were accepted, and since `deny` rules
  compared the path as written while the loader cleaned it, they got
  past `deny` (decision 0095). The loader and `deny` now both use the
  cleaned path, and `XO0158` is the only error such an import gets
  besides a `deny` match.
- Spec: the grammar defines `PathSeg` (ASCII letters, digits, `_`, `-`,
  `.`), matching the lexer; section 4.1 states that dashes outside the
  last segment need nothing (`use github.com/xo-lang/xo/lexer`). No
  behavior change.
- Real-time functions (spec 6.5, decision 0133): `uses realtime` on a
  function makes the checker reject allocation (XO1401), loops without a
  bound (XO1402), calls of functions that are not real-time, function
  values, and interface methods (XO1403), recursion (XO1404), and
  blocking (XO1405). Native builds defer the frees of a real-time
  section to its end (a 256 entry queue per thread, `XO_RTSTATS=1`
  counts) and fault with XO-F007 when a section allocated. Demo
  `examples/realtime-control`: the tick that drops a 20,000 point
  schedule writes its output in about 0.2 us instead of 30 to 114 us
  (bench/results/realtime-2026-10-10.md).
- Loop bounds: `for x in xs bound N { }` and `while c bound N { }` on
  every backend; a constant range longer than N is XO1402, otherwise
  iteration N + 1 faults with XO-F006, and `xo test` fuzzes functions
  with bounded loops.

- Native TLS: ECDSA P-256 signing and verification run on fiat-crypto's
  verified field arithmetic (vendored at a pinned commit,
  `third_party/fiat-crypto`, decision 0134) instead of Mbed TLS's bignum
  code: signatures byte identical, 4.8 times faster (23 us), a full
  HTTPS handshake on the native server takes 251 us of CPU (was 354; Go
  backend 227).
- `bench/perf/http/run.sh` takes `CONFIGS=xo-base-llvm` with
  `XO_BASE=<xo binary>` for before and after runs of the LLVM backend.
- `xo build --hybrid`: loops in Go assembler poll for preemption at every
  back edge (decision 0131), so a long hybrid loop no longer holds up the
  GC's stop the world or other goroutines on its P (11.6 s to 0.058 ms
  during an 11.5 s loop); the hybrid benchmarks are 0 to 7 percent slower
  than before (median 2), still 9 to 31 percent faster than the Go
  backend.
- `xo build --hybrid` compiles loops of programs that start tasks (was
  "no assembly": an RC verifier error on the task runner).
- `bench/perf/hybrid/run.sh` takes `XO_BASE=<xo binary>` to compare two
  compilers' hybrid builds in the same interleaved runs.
- The parser written in Xo (`selfhost/parser`, decision 0127): the whole
  grammar, built with the Go, arm64, and LLVM backends; for every file the
  Go parser accepts it prints the same syntax tree dump, and it rejects
  every file the Go parser rejects (repository, std, spec blocks,
  generated programs, token mutations). Its LLVM build parses 1.3x faster
  than internal/parser (bench/results/selfhost-parser-2026-10-10.md).
- Hidden `xo astdump [--sig] [--bench=N]`: the canonical syntax tree dump.
- arm64: functions with frames up to 28 KiB build (probed a page at a
  time; was a "frame too large" error past 4000 bytes), and values of
  80 or more words no longer fail to assemble (decision 0128).
- The program generator of the compile property test is the package
  internal/progen.
- Native test binaries now report test_once, test_logs, and
  test_shutdown byte for byte as the Go backend (`TestNoGCTests` runs
  them): a fault in a `task.once` function is stored and raised again
  in every caller; `log.configure(os.stdio, ...)` in a test writes
  records to the fake standard error (still kept for `testing.logs()`);
  `os.proc.set_grace` before `testing.run_main` sets that run's grace
  period, and a third signal makes `run_main` return 130 (with
  `exit_code()` `Some(130)`) instead of ending the test binary.
- `==` on `http.Request` works on the native backends (was XO1201): it
  compares the six spec fields and ignores typed request values, as on
  the Go backend (decision 0112 note; internal/llvm golden `request_eq`).
- internal/llvm `TestGolden` runs `fault_defer_chain` and `request_eq`
  under `-short` too.
- Tests for items that had none: native TLS session resumption
  (`TestTLSDiff`, TLS 1.3 and 1.2), decision 0118's reader handoff,
  scope pruning, and magazines (std/llrt/testdata/tasks.c), and decision
  0098's binary trimming (internal/build `TestGoBinaryTrimming`).

## v0.9.0 (2026-10-10)

Spec draft 0.9.

- Module proxies (decision 0126): `XO_PROXY` lists proxy URLs, `direct`
  (git), and `off`, with Go's fallback rules (comma after 404/410, pipe
  after any error; default `direct`), and `XO_NOPROXY` patterns go to
  git. The read only protocol serves `@v/list`, `.mod`, `.zip`, and
  `.lock` per module; `xo mod proxy serve [--fetch] [<cache dir>]`
  serves a module cache and can fill it from git, as a team cache.
  Proxied trees are checked against xo.lock exactly as git fetches are.
- Checksum log (decision 0124): with `XO_SUMDB=<name>+<id>+<key>
  [<url>]`, the first fetch of every module version must match the
  log's record, be in its signed Merkle tree, and extend every tree the
  module cache saw before; otherwise `XO1313` and nothing is cached. An
  unreachable log or malformed settings is `XO1314`; `XO_NOSUMDB` keeps
  private modules out. The default stays `off` (no public log yet).
- Definition hashes (decision 0125, the last step of 0086): `xo query
  hash <name>` prints a content hash per top level definition that
  survives renaming locals and changes with what the definition, or one
  it calls in the module, computes. `xo publish --check` lists the
  definitions (private ones too) changed since the last tag, including
  those changed only through a helper, next to the API comparison.

- std/http speaks HTTP/2 over TLS on every backend (decision 0121):
  `with_tls` servers and `https` clients agree on `h2` by ALPN, plain
  `http` stays HTTP/1.1, and nothing in the API changes. Requests of a
  connection run at once, each as a task; a fault in a streamed body
  resets only its stream. The native backends implement it in C (frames,
  HPACK, flow control) with net/http's limits (250 streams, the header
  list limit with 431, the rapid reset limit) and its error texts, and
  their client keeps HTTP/2 connections for later requests as Go's does.
  Over one connection the native server does 87.8k requests per second
  in the HTTP benchmark, the Go backend 58.2k
  (`bench/results/http2-2026-10-10.md`).
- Native test binaries (`xo test --no-gc`, decision 0120): property tests
  and `testing.gen` work, with the Go backend's generated values for a
  seed, the same failing case, and the same shrunk input in the report.
  A `Set` or `Map` keyed by a sized integer (`Set[Int8]`) now builds
  natively. `xo test --real` gives native tests real capabilities for
  the effects they declare (`fs`, `fs.read`, `fs.write`, `net`, `clock`,
  `env`, `stdio`, `proc`), as on the Go backend.
- Native backends: deferred code that faults no longer skips the rest
  of the cleanup. The deferred code registered before it and that of
  the blocks around it runs, the last fault is reported, as on the Go
  backend (core 3.5, decision 0085 amendment), and a task that caught
  such a fault with `task.isolate` sees cancellation again (it could
  deadlock before).
- LLVM backend (decision 0119): generated code is compiled for the same
  CPU and features as the C runtime (clang's defaults): LSE atomics for
  shared reference counts on darwin/arm64, outline atomics (LSE when the
  CPU has it) on Linux arm64. HTTP benchmark +3.5 percent.
- Native `Mutex` (decision 0118): a writer's release hands the lock to
  the readers waiting, so they no longer wake only to park again: a
  contended readers and writer load (`bench/perf/conc` `rwmutex`, new)
  went from 967 to 131 ms and from 6.3 to 0.25 s of CPU. Long lived
  scopes (the native HTTP server's connections) no longer keep every
  finished task until they close, and the native allocator moves freed
  blocks between threads, so a server's memory stays flat (28 MB instead
  of 295 MB after 300k TLS connections).
- Native HTTPS (decision 0117): full TLS handshakes about 1.6 times as
  fast and at half the CPU (9.7k to 15k per second in the HTTP benchmark,
  Go backend 17k): X25519 key shares through HACL*, faster P-256
  inverses, a spinning lock for Mbed TLS's key store, SHA instructions.
  The native server now sends session tickets as Go's does (TLS 1.3
  resumption with ECDHE, TLS 1.2 tickets; keys rotate daily), so
  returning clients skip the certificate signature. `bench/perf/http`'s
  loadgen has `-resume` (`RESUME=1` in run.sh).
- `xo lsp` (decision 0114): a Language Server Protocol server with
  diagnostics and their fixes as quick fixes, hover (signature, doc, or
  type), definition (into std too), references, rename, document
  symbols, semantic tokens, and formatting. The VS Code extension starts
  it; ```xo fences in Markdown are highlighted.
- VS Code syntax highlighting (decision 0113): `editors/vscode/` registers
  `.xo` files with a TextMate grammar covering spec section 1 (keywords,
  contextual words, interpolation, raw and bytes strings, durations, sets,
  effects in `uses`). Install by symlinking it into `~/.vscode/extensions`.
- `use c "<header>" [link "<lib>"]... [as name] [uses ...]` on the
  native backends (decisions 0093 stage 2 and 0116): bindings generated
  from the header with clang (`xo bind --c`), scalars, Str, Bytes with a
  length, flat structs, and opaque handles at the boundary, the `ffi`
  effect unless the line narrows it, XO1104/XO1105 for headers and names
  that do not bind, XO1201 on the Go backend. Calls are not recorded
  yet (native builds do not record).
- Decision 0111: passing one variable, or overlapping places (`x` and
  `x.f`, `xs[i]` and `xs[j]` unless both indexes are different
  literals, a `var` receiver and an argument naming it), to two `var`
  parameters of one call is `XO0503` on every backend; the native
  backends no longer report XO1201 for it.
- Decision 0112: a value that holds a handle (`Mutex`, `Atomic`, `Chan`,
  `Task`, `Scope`, `Listener`, `Conn`, `task.Group`, the std/http and
  std/sql handles, the test fakes) or a function value is not
  comparable: `==`/`!=`, Map keys, Set elements, `derive Eq`/`Hash`, and
  `T: Eq`/`Hash` arguments are `XO0404` with a note naming the field
  that holds it; `derive Ord` already was. The checker now looks into
  struct fields and enum payloads for `Eq` (it looked only at type
  arguments).
- Native backends (decisions 0107 to 0109): `Proc.run`, `os.tasks` (a
  scope around main and around each test), method values
  (`let f = x.method`), `map_concurrent_until`, base64,
  `Range.to_list`, `Rand.shuffle`, `target` (a compile time constant),
  and in native test binaries MemFs (every method, `fail_next`, case
  sensitivity, `files`), FakeProc `run` and `on_run`, `os.proc.exit`
  inside `testing.run_main`, and `run_main` of a main that cannot fail.
  Fixed natively: a receive arm after an `after` arm lost its value; a
  `?` in a closure printed the function's frame name without
  `.closure`, and one in a `Mutex.with` closure none;
  `FakeClock.advance` returned before the tasks it woke ran; the
  success type of `Err(e)?` was XO1201. One binding passed to two `var`
  parameters is now XO1201 natively (the Go backend shares it, the
  native backends copied it).
- TestCompileProperty (decision 0110): random checker valid programs over
  the optional syntax must build on the Go backend and with `--no-gc`,
  and run alike.
- Fixes from Codex r3: a `select` arm that receives without binding the
  value no longer generates Go that fails to compile; the "propagate with
  ?" fix is offered only where `?` can propagate; help notes for XO0205
  in closures and for passing `TestOs` where `Os` is expected.
- Native backends (`--no-gc`, LLVM and arm64): interface types as values
  (decision 0100): dynamic dispatch, adapters for methods that cannot
  fail, error conversion, interface to interface conversion, Display and
  Debug of the dynamic value, and `==`; `x.debug()` on any value.
- Native backends: newtypes (`Email("x")`, `e.0` read and assigned,
  `Email(p)` patterns, Debug, `==`, Map keys) and list patterns
  (`[]`, `[a, .., z]`). Fixed: an or pattern whose alternatives bind the
  same name (`Leaf(v) | Node(left: v, ..)`) read the first alternative's
  binding natively, a stale value when another alternative matched.
- Native backends (decision 0101): Float32, `Secret[T]` redacted in every
  output path, `Buf[T]`, `<` on `derive Ord` types, `Int32.try` and the
  other sized `try`s, `Int.pow`, `clamp`, `min`/`max` on Str, Set union,
  intersect, difference, `Map.filter`, `List.insert`, `remove_at`,
  `indexed()`, `xs + ys`, std defaults (`crypto.token(r)`,
  `Chan.new()`), `get_or_fault` on any key type, builtin collections
  through interfaces, shorthand field patterns. `==` on a value holding
  a function or runtime handle outside an interface is now XO1201
  natively (it failed in code generation). New test
  TestGenGoldensNative runs 66 internal/gen goldens on Go, LLVM, and
  arm64 and compares them.
- Native backends: `std/path` (std/native/path.xo), `task.semaphore`,
  `task.once` (a fault in its function is not stored natively: the next
  caller runs it again), and `task.map_concurrent`
  (std/native/task.xo); Debug text of `Result[T, Never]` values. Fixed:
  natively, a task whose value is a Result failed its scope when that
  value was an `Err`; shorthand struct patterns `Bag{items}`.
- Native backends (decision 0102): every Fs method and attenuation
  (`sub`, `cwd`, `read_only`, `write_only`, escapes `Denied`),
  `Stdio.write`, and standard input reads (`read_line`, `read_all`,
  `lines`) that a cancellation or a stop request ends with
  `IoErr.Canceled`. Fixed natively: `task.with_timeout` returns
  `TimedOut` when its closure finishes after the deadline without a
  cancellation point, as on the Go backend.
- Native backends: error frames (decision 0104): each `?` records the
  function, location, and parameters lazily, and main's error report
  prints `  at ...` lines identical to the Go backend's.
- Native Fs and standard input code is compiled into a program only when
  it calls them (XO_FS, XO_STDIN; hello world unchanged). Fixed: hybrid
  builds (`--hybrid`) declared a Bool result's argument size rounded up
  (go vet -asmdecl).
- `xo test --no-gc`: `FakeStdio.set_input` feeds the reads; fixed a hang
  after `FakeProc.signal_after` fired (the virtual clock stopped moving).
- Native backends: all of `std/time` (decision 0094; std/native/time.xo
  and std/llrt/xo_time.c, a port of Go's TZif reader and zone lookup):
  zones with the same per use embedded data (or `--tzdata=system`), civil
  time, gaps and overlaps, layouts, strict parsing with the same error
  texts and faults; also `Duration.from_secs`, `scale`, `ceil_seconds`,
  `Time.parse_rfc3339`, and `Time.UNIX_EPOCH` as a value. Programs that
  do not use std/time do not grow. TestTimeDiff compares about 70,000
  lines with the Go backend. Fixed: the arm64 backend failed to assemble
  functions over 8192 instructions (`tbz` range).
- HTTPS in std/http on every backend (std.md 4.6, decisions 0105 and
  0106): `http.tls_cert(cert_pem, key_pem)`, `Server.with_tls(certs)`
  (SNI picks among the certificates), `Client.with_roots(pem)`, and
  `TlsErr`. The native backends (`--no-gc`) speak TLS 1.2 and 1.3 through
  Mbed TLS 4.1.1 (vendored in `third_party/mbedtls`, built once per
  compiler into the user cache, linked only into programs that use
  std/http), verify against the system's roots on macOS and Linux, and
  report Go's error texts; an `https` URL is no longer `XO1201` there.
  std/http speaks HTTP/1.1 only, also over TLS: the Go backend's client no
  longer negotiates HTTP/2. Failed TLS handshakes are not logged.
- `bench/perf/http/run.sh`: `SCHEME=https` serves the Xo configs over TLS
  and `NEWCONN=1` opens a connection (and a handshake) per request.

## v0.8.0 (2026-10-09)

Spec draft 0.8.

- Modules (decision 0086 steps 1 and 2): `require <path> <version>` of Xo
  modules, minimal version selection, git fetch into a read only module
  cache, `xo.lock` hashes verified on every build, `xo get`, `xo mod
  tidy`, `xo vendor`, `xo mod clean`, `--offline`, and `xo.work`
  workspaces (`--workspace=off`). Diagnostics XO1301 to XO1308.
- Codex r2 first run failures: `XO0175` for several match arms on one
  line has a split fix and no longer cascades into a false `XO0301`;
  `.display()` works on every `Display` type (derived, std errors,
  builtins); a closure that never completes infers `Never` instead of
  `XO0402`.
- Effect ceilings and checked semver (decision 0086 steps 3 and 4):
  `require <path> <version> uses none` in xo.mod and `use <dir> uses ...`
  in xo.work (XO1309 at a call above the ceiling), API and effect diffs
  from `xo get` (and `xo get -u`), `xo publish [--check]` against the
  previous tag (XO1310, XO1311; prints the tag command, never pushes),
  `xo init`, `/vN` major version module paths, and `xo serve` noticing a
  new or changed xo.work above the directories.
- `std/time` (decision 0094, Go backend): zones with per use embedded
  data (`--tzdata=system`), civil dates and times with explicit daylight
  saving gaps and overlaps, layouts with named fields checked at compile
  time (XO0420; XO0177 for `{yyyy}`, `%Y`, `2006-01-02` habits).
- Import direction rules (decision 0095): `deny <pkg> -> <pkg>, ...`
  lines in xo.mod with `/...` patterns, checked at every `use` line
  (new code XO1312; a rule naming no package is XO1301), kept by `xo mod
  tidy`; `xo query imports` and `xo query deps` list the `use` lines
  (also in the MCP `query` tool).
- std/time error text (std.md 13): `ZoneErr` and `CivilErr` display as
  sentences (`unknown time zone "Mars/Olympus"`, `2026-03-08T02:30:00
  America/New_York does not exist (skipped by a daylight saving
  change)`) instead of the derived `Skipped(at: ...)` form, matching
  `ParseErr` and `LayoutErr`.
- Newtypes (decision 0097, spec audit gap 4): `e.0` reads the underlying
  value and `Email(s)` destructures it in `let` and `match`.
- `xo check --fix` (decision 0096): applies the unambiguous machine fixes
  (as `xo fix`), reformats, then reports what remains plus an `applied`
  list; the MCP `check` tool takes `fix`, and the bench harness agent
  loop uses it while still scoring first runs as written and after fix.
- Go backend output (decision 0098): hello world 7.9 to 3.6 MB, Go build
  cache growth per program 34 to 2.9 MB, compile memory 286 to 123 MB.
  Binaries carry no DWARF or symbol table (`XO_GO_DWARF=1` keeps them for
  a debugger); programs that do not import std/http do not link net/http.
- Spec audit second pass: `x.debug()` works on every value (std.md 1;
  was XO0601 on concrete types); new tests for single field positional
  variants, `Secret` redaction in Debug and log fields, `target` at run
  time, `Stdio.write`, `Fs.cwd`, `Fs.case_sensitive`, a real `Proc.run`,
  `XO-F005`, `xo test --real`, and byte identical rebuilds (spec 12.1 now
  says "in the same directory").
- Modules in repository subdirectories (decision 0086): a module path
  `github.com/acme/tools/cli` is the directory `cli/` of the repository
  `github.com/acme/tools`, versioned by tags `cli/vX.Y.Z` (as Go);
  fetch, MVS, xo.lock, `xo vendor`, `xo get`, and `xo publish [--check]`
  (prefixed tag command) handle it; nested modules are left out of a
  module's tree and hash.
- Go calls in record and replay (decision 0093 stage 1): `--record`
  records `use go` calls whose arguments and results are values (an
  argument hash and the results) and `xo replay` answers them without
  calling Go; calls with a Go handle or a callback run again and the
  replay reports ``re executed foreign call `<name>` ``; pure std
  packages and xo.mod `replay rerun go <path> [names]` lines are not
  recorded. Binding format v5; traces gain `"foreign": 1`, and older
  traces still replay (every Go call runs again).
- Release builds (decision 0099): `xo build --release` compiles out
  `requires(debug)` and `ensures(debug)` on every backend and reports
  XO1308 for workspace overrides; `--ci` does not imply it.
- Lazy error frames (decision 0092, Go backend): frames keep copies of
  the parameter values and format them, bounded to 64 bytes, only when a
  trace is printed. 1,000,000 errors through 5 frames: 0.93 s to 0.10 s
  with short parameters, 2.78 s to 0.10 s with 90 byte ones; error output
  unchanged. `xo build --release --error-args=false` drops the values.

## v0.7.0 (2026-10-09)

First tagged release. Spec draft 0.7.

- Language: request scoped arenas by default, `use go` packages, two
  phase shutdown, `Mutex.with_read`, `BytesBuilder`, rune and integer
  conversions, native contracts and fault unwinding, habit diagnostics
  (XO0177), `clock.sleep_or_cancel`, cancellable stdin, tuple JSON,
  `Duration.parse` (decisions 0041 to 0088).
- Backends: Go (default), LLVM, native arm64, `--no-gc`, `--hybrid`.
- Toolchain: `xo version`.
- Planned before 1.0: modules and workspaces (0086), `std/time`, native
  TLS, and decisions on the Appendix B questions.
