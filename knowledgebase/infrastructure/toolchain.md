# Toolchain and layout

Last verified: 2026-10-08

## Host

- Development host: macOS (darwin/arm64).
- Go: 1.26.1 (`go version`). The P1 prototype is written in Go and emits Go.
- Git repository, remote `git@github.com:josephspurrier/xo.git`, branch `main`
  (initialized 2026-10-08). `.gitignore` excludes `.tools/`, build outputs
  (`benchtask`, `*.exe`, `*.test`, `bin/`, `dist/`), logs, and editor files.
- `go test ./...` needs a C toolchain for the race detector tests (skipped
  when `go env CGO_ENABLED` is 0).
- clang (2026-10-08): Apple clang 21.0.0 at `/usr/bin/clang` (Xcode 26.5
  toolchain), used by the LLVM backend prototype (`xo build
  --backend=llvm`, see "LLVM backend (P3 prototype)"). `llc` and `opt` are
  not installed; clang compiles `.ll` files directly. The Command Line
  Tools SDK that clang uses by default (MacOSX27.0.sdk) does not link
  with this linker; the build falls back to the Xcode SDK
  (`gotchas/macos-clt-sdk-link-failure.md`). The LLVM tests skip when no
  clang is found (`$XO_CLANG`, then PATH).
- cc (2026-10-08): `/usr/bin/cc` is the same Apple clang 21.0.0. The
  native arm64 backend (`xo build --backend=arm64`, see "arm64 backend
  (native)") uses it to assemble the generated `.s` and compile and link
  `xo_rt.c` (`$XO_CC`, then `cc`, then `clang` on PATH), with the same SDK
  fallback as the LLVM backend. Last verified: 2026-10-08.

## Go module

- `go.mod` at the repo root: `module github.com/xo-lang/xo`, `go 1.26`.
  The module path intentionally stays `github.com/xo-lang/xo` even though the
  git remote is `github.com/josephspurrier/xo` (owner's choice, 2026-10-08).
  So `go install github.com/josephspurrier/xo/cmd/xo` does not work; build
  from a checkout with `go build -o .tools/bin/xo ./cmd/xo`.
- Standard library only. No third party module dependencies, for the
  compiler or for generated programs. (Go packages bound with `use go` are
  dependencies of the generated program, not of the compiler; see "Go
  interop".) Last verified: 2026-10-08.
- Vendored third party code (decision 0059): `std/rt/xcrypto_argon2.go`
  and `std/rt/xcrypto_blake2b.go` hold the portable Go of
  `golang.org/x/crypto` v0.55.0 packages `argon2` and `blake2b` (no
  assembly, no `x/sys/cpu`), renamed into package `rt`, with the Go
  authors' BSD 3 clause license in each file header. They are not a module
  requirement: the generated `go.mod` stays `module xoprog` plus `use go`
  requirements, so builds need no module download for any `--targets`
  entry. Why not a dependency: `rt` is copied whole into every build, so a
  requirement of any `rt` file is one of every program, and with an empty
  module cache and no network the build fails (`module lookup disabled by
  GOPROXY=off`; gotcha `runtime-import-is-every-programs-dependency.md`).
  To update: diff a newer x/crypto release against these files and rerun
  `go test ./std/rt -run 'Argon2|Blake2b'` (RFC 9106 and RFC 7693 vectors,
  plus keys recorded from x/crypto in `std/rt/testdata/argon2id_xcrypto.txt`).

## Commands

Run from the repo root.

```
go build ./...                           # build the compiler packages and std
go test ./...                            # all tests (lexer, parser, checker, spec examples, corpus, CLI, golden programs, race, cross compile)
go vet ./...
gofmt -l internal cmd std                # must print nothing
.tools/bin/staticcheck ./...             # must print nothing
go run ./cmd/xo parse [--json] [--ast] [--sig] <files or dirs...>
go run ./cmd/xo check [--json] <dirs...>
go run ./cmd/xo build [--targets=all|os/arch,...] [-o out] [--json] [--no-mono] [--arenas] [--backend=go|llvm|arm64] [dir...]
go run ./cmd/xo run [--seed=N] [--record=trace.xotrace] [--holes] [--no-mono] [--arenas] [--backend=go|llvm|arm64] [dir] [-- args...]
go run ./cmd/xo replay [--dir=<module dir>] <trace.xotrace | crash.xocrash>   # rebuild and rerun offline from the trace (core 11.3)
go run ./cmd/xo test [--seed=N] [--explore=N] [--run=text] [--race] [--real] [--cases=N] [--json] [-v] [--holes] [--no-mono] [--arenas] [dir...]
go run ./cmd/xo dump [--timeout=5s] <pid>                        # task tree of a running Xo program (core 11.4)
go run ./cmd/xo bind [--dir=<project dir>] [--json] <go import paths...>   # bind Go packages for `use go` (Go interop)
go run ./cmd/xo fmt [--check] [--json] [files or dirs...]        # canonical layout (core 10.5)
go run ./cmd/xo vet [--json] [dirs...]                           # advisory checks, never errors
go run ./cmd/xo fix [--dry-run] [--json] [--from=draft-0.N] [--edition=YYYY] [dirs...]  # apply check fixes, migrate
go run ./cmd/xo query <kind> [--json|--text] [--dir=<dir>] <arg> # type, def, callers, effects, holes, api (core 10.3)
go run ./cmd/xo edit [--dry-run] [--dir=<dir>] <op> <args...>   # rename, add-field, change-sig (core 10.4); JSON summary, exit 1 when rejected
go test ./internal/edit -run TestGolden -update          # rewrite internal/edit/testdata/*.golden
go run ./cmd/xo doc [--json] [--all] [--dir=<dir>] [module]      # Markdown docs from doc comments
go run ./cmd/xo explain [--json] <code>                          # long form explanation of a diagnostic code
go run ./cmd/xo mcp                                              # MCP server on stdio for agents (see "Agent interfaces")
go run ./cmd/xo serve [--socket=path] [stop|status]              # resident checker; check and query use it when it answers
go test ./internal/format -run TestGolden -update        # rewrite internal/format/testdata/*.golden
go test ./internal/gen -run TestGoldenNoMono -golden-update   # rewrite want.txt of the golden programs
go test ./internal/llvm -run TestGolden -golden-update   # rewrite want.txt of the LLVM backend golden programs (from the Go backend's output)
go test ./internal/ssa -update                       # rewrite the SSA goldens (testdata/golden/*/want.ssa and want.rc.ssa, testdata/bench/*.ssa and *.rc.ssa)
XO_SSA_DUMP=/abs/dir [XO_SSA_RC=1] go test ./internal/ssa -run TestDump -v   # print a program's SSA (after InsertRC), or every builder and verifier error
XO_LEAKCHECK=1 ./prog                                  # an LLVM backend binary prints `xo leakcheck: N live of M allocations` at exit
go test ./internal/llvm -run 'TestRuntimeThreads|TestGoldenThreads'   # shared counts and the runtime under ThreadSanitizer (decision 0058; about 10 s)
clang -O1 -g -fsanitize=thread std/llrt/testdata/rc_threads.c -o /tmp/rct && /tmp/rct   # the C stress test by hand (add -isysroot <SDK> when the default SDK does not link)
go test ./internal/arm64 -run TestGoldenAsm -update     # rewrite the golden assembly internal/arm64/testdata/{fib,sieve}.{darwin,linux}.s
go run ./cmd/xo build --backend=arm64 --targets=linux/arm64 .   # emit <build dir>/arm64/prog-linux.s (linked only on a linux/arm64 host)
XO_RANGE_SEEDS=25 go test ./internal/gen -run TestRangeDifferential   # slow: more random programs, range analysis vs all checked
XO_LIST_SEEDS=40 go test ./internal/gen -run TestListModel   # slow: more random List value semantics programs vs a deep copy model (XO_LIST_DUMP=<dir> keeps them)
go test ./internal/gen -run TestStdTypes -update        # regenerate std/rt/std_gen.go from std/*.xo
go test -short ./...                                    # about 10s: skips race, cross compile, CLI build, replay, and the --no-mono and --arenas golden lowerings; use per commit, run the full suite before merging (about 2.5 min)
go test -race ./std/rt -run 'TestMutex|TestSched'      # runtime lock and scheduler under the race detector (Mutex.with_read readers, writers, cancellation; last verified 2026-10-08)
go test ./std/rt -run '^$' -bench . -benchmem           # runtime micro benchmarks (Map put/get/update/iter/copy/fork, Set, Eq, Hash)
bench/perf/run.sh                                       # Xo vs Go collection, List write, and generic workloads (see Benchmark tooling)
MONO=1 bench/perf/run.sh                                # Xo --no-mono and Xo (default, --mono) columns
bench/perf/http/run.sh                                  # task 01 HTTP load: Go vs Xo vs Xo --arenas (see Benchmark tooling)
bench/perf/llvm/run.sh                                  # compute and string workloads: Go vs Xo (Go backend) vs Xo --backend=llvm (RSS=1: peak memory table)
bench/perf/llvm/native.sh                               # same workloads: Go vs Xo --backend=llvm vs Xo --backend=arm64, time and peak RSS (arm64 host)
go test -run '^$' -fuzz=FuzzParse -fuzztime=60s ./internal/parser/   # optional fuzzing
go test ./internal/check -run TestGolden -update          # rewrite golden diagnostics after an intended change
go test ./internal/check -run TestCorpusV03 -corpus-report # rewrite the generated counts in bench/results/check-corpus-v0.4-spec.md
go test ./internal/check -run TestCorpusMutations -mutations -v   # slow: plant mutations in the corpus, count detections
.tools/bin/xo check examples/notes-service && .tools/bin/xo test examples/notes-service && .tools/bin/xo build --targets=all examples/notes-service   # example app; not run by go test (last verified 2026-10-08)
```

`./...` is safe since `bench/go.mod` made `bench/` its own module (see
`gotchas/root-module-absorbs-bench-go.md`). Last verified: 2026-10-07.

`xo parse` walks directories for `.xo` files, prints diagnostics in a
`file:line:col: error XOnnnn: message` form on stderr (plus a summary line),
and exits 1 if any error was reported, 2 on usage errors. `--json` prints
`{"files": [...], "diagnostics": [...]}` on stdout with diagnostics in the core
10.2 shape (notes and fixes are always arrays). `--ast` adds each file's syntax
tree (every node has `kind` and `span` "line:col-line:col"). `--sig` accepts
function declarations without bodies (spec/std.md style signature listings).

`xo check` treats every directory under the given directories that holds
`.xo` files as a module (`testdata` and dot directories are skipped), loads
the user modules they import through the nearest `xo.mod` (`module <path>`:
`use <path>/<dir>` maps to `<xo.mod dir>/<dir>`), and loads std modules from
the embedded declaration files in `std/`. It prints `file:line:col: severity
XOnnnn: message` lines (with notes and fix titles) and a summary on stderr, or
with `--json` `{"modules": [...], "diagnostics": [...]}` on stdout. Exit status
is 1 when any error is reported (warnings such as XO0900 holes do not count),
2 on usage errors.

`xo fmt` (2026-10-08, `internal/format`) prints each file from its syntax
tree in the one canonical layout (rules at the top of
`internal/format/format.go`): 2 space indentation; one statement,
declaration, member, or arm per line; blank line runs collapse to one; a
bracketed list is split one element per line with a trailing comma when
the source breaks the line right after the opening bracket, else joined
on one line without one; a body stays on one line only if it was on one
line with at most one statement (struct, enum, and interface bodies with
any number of members); breaks the grammar allows mid expression are kept
(before `.` in a chain, after a binary operator, after `=>`, before `->`
and each clause of a function header), with continuation lines one level
deeper; `} else {`, `} invariant`, `} derive` on the brace line;
comments kept, trailing comments of consecutive lines aligned; literals
copied byte for byte. It writes changed files in place and prints their
names; `--check` changes nothing, lists unformatted files, and exits 1
when there are any; files with syntax errors are reported and skipped
(exit 1). Dot directories are skipped. Files that only parse as signature
listings (`std/*.xo`) are formatted in that mode. Tests:
`internal/format` `TestLayout`, `TestGolden` (`testdata/tricky.xo`),
`TestSpecExamples` (every core.md example that parses), and `TestCorpus`
(temporary copies of bench/xo, bench/paper, internal/check/testdata,
internal/gen/testdata, and std: each file must parse to the same tree
without spans, comments are kept, a second format is a no op, and each of
110 programs must type check with the same diagnostics, code, severity,
and message, before and after). `xo build --ci` does not exist yet.

`xo vet` (2026-10-08, `internal/vet`) checks like `xo check` and reports,
for the root modules, only warnings and notes (exit 0; check errors are
counted on stderr, `check_errors` in JSON): the check warnings (XO0900,
XO0305, XO0804) plus XO0703 (an effect written in a `uses` clause that
the body never performs; a sub effect counts for its parent and the
parent for a sub effect; implicit effect variables are never reported),
XO0207 (`x ?? [].f()`: a selector chain on the right of `??` whose base is
a literal, with a fix that parenthesizes the `??`), XO0208 (note: every
prelude `.ignore_err(reason)` call with its reason and the Result type),
XO1103 (note, one per module: `use go` packages and the functions whose
performed effects include `go`), XO0901 (`dbg(...)` with a fix that keeps
the argument). Human output goes to stdout, the summary to stderr.

`xo fix` (2026-10-08, `internal/fix`) first runs the migration rewriters
selected by `--from=draft-0.N` (also `0.N`, `v0.N`; every rewriter whose
change came in a later draft), then loops: check the sources through
`check.CheckOverlay` (rewritten contents in memory, nothing written yet),
apply the first fix of every diagnostic whose edits do not overlap one
already taken this round (a fix is all or nothing; two inserts at the same
point conflict), and repeat until a round applies nothing, the sources
return to an earlier state, or 25 rounds. Changed files are reformatted
with `xo fmt` when they parse, then written (not with `--dry-run`). Output:
one `file:line: rule: title` line per change and a summary; `--json` gives
`{dry_run, changes, files, rounds, remaining}`. Exit 0 even when errors
remain (listed on stderr), 2 on usage errors. Rewriter registry
(`fix.Registry`, run in order): `variant-case` (0.2: `ok( err( some(` and
bare `none` to `Ok Err Some None`; skips `.ok()`, `fn ok(`, `let err`, and a
variable `err` not followed by `(`), `scope-timeout` (0.2: `scope s timeout
d { b }` to `task.with_timeout(clock, d, fn() { scope s { b } })` plus `use
std/task`, only in files that name `clock`; elsewhere reported as skipped),
`interp-dollar` (0.3: `{x}` to `${x}` in plain and triple quoted strings
when the content starts with a letter, digit, `_`, or `(` and holds no
quote), `discard-reason` (0.4, the XO0206 fix), `map-get-or-fault` (0.6,
the XO0410 fix added to the checker: `m[k]` to `m.get_or_fault(k, "TODO:
why the key must exist")`). Code based rewriters are applied by the loop
anyway; the report credits them as `name (XO0nnn)`.

Measured 2026-10-08 on copies of `bench/paper` (originals untouched),
modules with no check errors before and after `xo fix --from=<draft>`:
v0.1 0/5 to 0/5 (135 errors to 36; the rest are API drift such as
`std/test`, `http.Server.handle`, `?` on `Fs`), v0.2 0/5 to 4/5
(04-file-sync keeps 9 XO0401 `?` on `Fs`), v0.3 5/8 to 8/8.

Editions (2026-10-08): `edition YYYY` in xo.mod is recorded on
`check.Module.Edition` (`check.CurrentEdition`, 2026, without the line or
without an xo.mod). An edition outside `check.Editions` or a malformed
line makes `xo check` fail with a usage error (exit 2) naming the xo.mod.
Rules that change at an edition boundary are entries of `editionRules`
(`internal/check/edition.go`, `Since` edition plus a per file check), run
after `checkModule` for modules with `Module.AtLeast(Since)`; the list is
empty for 2026 and `TestEditionRuleHook` exercises it with a fake 2027.
`xo fix --edition=YYYY` only validates the edition until a second edition
exists.

`xo query` (2026-10-08, `internal/query`) checks `--dir` (default `.`; for
`type`, also the file's directory when it lies outside) and answers:
`type file:line:col` (innermost typed expression or binding pattern; the
natural type too when an implicit conversion applied), `def <name>`
(`name`, `Type.member`, `module.name`, `module.Type.member`, `module` a
local name such as `http` or an import path; functions, methods, statics,
types, constants, fields, variants; signature printed by
`format.Decl`), `callers <fn>` (calls and uses as a value in user
modules, with the enclosing declaration), `effects <fn>` (declared or
inferred set, and each performed effect with its call chain, the
XO0701 note chain, `check.Info.Performed`), `holes` (the XO0900
diagnostics), `api <module path or dir>` (pub declarations outside test
files; struct signatures show only pub fields and no invariant). JSON is
the default when stdout is not a terminal (`--text` forces text);
unknown names exit 1, usage errors 2.

`xo edit` (2026-10-08, `internal/edit`, decision 0044) checks `--dir`
(default `.`), resolves the target with `query.Session.Objects`, collects
every reference from the checker's records (`Info.CallOf`, `Info.Ref`,
`Info.TypeOf` of struct literals, `Info.VarOf` of parameters), applies the
edits in memory, rechecks with `check.CheckOverlay`, and only then writes
all files (temporary file plus rename per file). Files that were in
canonical layout are reformatted. Always prints the JSON summary `{op,
target, ok, written, reason, files, edits, diagnostics, sites}`; exit 0
when applied (or checked with `--dry-run`), 1 when rejected (nothing
written), 2 on usage errors (unknown name, bad signature). Limits: rename
covers functions, methods, and constants, not types, fields, or interface
methods; change-sig takes no clauses (`uses`, `requires`).

`xo doc` (2026-10-08, `internal/doc`) prints Markdown for the root modules
under a directory (default `.`) or one module by import path (`--dir`):
an index, then constants, types (methods under their type), and
functions, each with its signature in an `xo` block, its doc comment
(example fences without an info string retagged `xo`), the number of
examples, and its position. `--all` includes non pub declarations,
`--json` emits the same data with `examples` (the bodies of the fenced
examples). Running the examples (core 1.2: `xo test`) is not implemented.

`xo explain <code>` (2026-10-08, `internal/diag/explain.go`) prints the
long form explanation of a code (`XO0201`, `xo0201`, or `0201`): title,
severity, detail, an example that triggers it, and the fix;
`TestExplanationsComplete` requires an entry for every code in
`codes.go`, and every example was run through `xo check` when written.
Last verified: 2026-10-08.

Diagnostic positions: lines and columns are 1 based, columns count bytes, and
spans are half open (`end_col` is one past the last character), which matches
the insertion fix example in core 10.2.

`xo build` checks (errors and holes block the build), generates Go, and runs
`go build -trimpath` with `CGO_ENABLED=0` per target; it prints one
`os/arch: path` line per binary. `--targets=all` is linux/amd64,
linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64, and
`wasi/wasm32` (Go's wasip1/wasm). Binaries go to `-o` (a file for one
target, a directory for several) or the build directory's `bin/`. `xo run`
builds for the host and runs with the arguments after the directory or
`--`, passing the exit status through. `xo test` builds the test blocks of
every root module into one test binary and runs it: it prints failures (and
passes with `-v`) and a summary line `ok: N passed, M failed, K skipped`;
exit status 1 when a test fails. Tests that declare `uses` run only with
`--real`, with real capabilities for the declared effects. Each test runs in
its module's directory (as `go test` does per package), in a fresh test
world (virtual clock, MemFs, FakeNet, ...) on the deterministic scheduler
(core 7.4, see "Deterministic scheduling and live inspection"), seeded
with the test's pinned seed (`test "x" seed N`), else `--seed`, else 1;
the FAIL line prints the seed. A test whose tasks are all blocked with no
timer pending fails with fault XO-F001 and the wait cycle (or every
blocked task) with locations; a test that runs over 60s of real time
fails. `--explore=N` runs each concurrent test that passed under its seed
again under the next N-1 seeds and reports the first failing one, with
how to reproduce and pin it. Property tests (`test "x" for v: T`) and contract
fuzzing (every function with `requires`/`ensures` in a root module, core
9.1) run 100 generated cases (`--cases=N`) and shrink a failing input.
Last verified: 2026-10-07.

Build directories live in `$XO_BUILD_DIR` or `<user cache>/xo/build/<hash of
the module paths, mode, lowering, `--arenas`, and layout tag>/` (stable, so Go's build
cache is reused; the layout tag keeps older xo binaries out, see
`gotchas/build-dir-shared-across-xo-versions.md`): `go.mod` (module
`xoprog`), `rt/` (the runtime, copied from the embedded `std/rt`),
`m/<module ident>/xo.go` per Xo module, `main.go`, `xo_adapt.go`
(package main: interface adapters registered for generic code, often
empty), and (build mode, since 2026-10-08) `xo_buildinfo.go` (package
main: `rt.SetBuildInfo` with the build id, source hashes, main module
directory, and options, `build.Info`). Last verified: 2026-10-08. Crash bundles go to
`$XO_CRASH_DIR` or the system temporary directory. `XO_GO` overrides the go
command (default: `go` on PATH).

## Deterministic scheduling and live inspection

Last verified: 2026-10-08. Code: `std/rt/sched.go` (threads, scheduler,
monitor, reports, dump), hooks in `task.go`, `scope.go`, `chan.go`,
`sync.go`, `once.go`, `world.go`, `stop.go`, `caps.go`, `http.go`,
`testing.go`, `main.go`; `cmd/xo/dump.go`.

- Threads: every goroutine that runs Xo code is a `thread` (main, test,
  spawned task, HTTP request, runtime helper); tasks on one goroutine (a
  scope body and its opener, an in memory request's handler task) share
  it. `Task.th`. A thread records what it blocks on (`blockOn(kind, obj,
  cancelable)`) for reports. Cancellation tree nodes carry a kind, their
  task, a creation sequence number, and (scope bodies) the opening site.
- Scheduler (`sched`): one thread holds the turn; scheduling points
  (`Task.Yield`) are channel operations (send, recv, try_send, try_recv,
  close, select), await, Mutex.with, Mutex.with_read, Semaphore.with,
  Once.get, spawn,
  Atomic operations, sleep, `task.is_canceled()`,
  `task.stop_requested()`, and the effect calls of the fakes (clock.now,
  stdio, MemFs, FakeNet) and of real clock and stdio. At each point a PCG
  seeded with the seed picks the next ready thread. A blocking thread
  gives the turn away; wakers move it to the ready list. In a test world
  the scheduler shares `World.mu` and drives the virtual clock (fires the
  next timer when no thread is ready, ends `FakeClock.advance`, and
  closes `World.stuck` when nothing can fire). Tests with real
  capabilities run without it. `xo run --seed=N` sets `XO_SEED=N` for the
  program (any built binary honors `XO_SEED`): the same scheduler with
  the real clock; real timers and I/O wake threads from outside, and
  blocking I/O (stdin reads, HTTP client and server waits, `proc.run`)
  leaves the scheduler while it waits. Interleavings are reproducible as
  long as timer and I/O timing does not decide them.
- Deadlocks (fault XO-F001): test worlds report when the scheduler or
  the virtual clock finds every thread blocked with no timer pending.
  Real runs have a `monitor` (`RunMain`): live threads versus threads
  blocked on another task (channel, mutex, semaphore, await, scope end,
  Once); waits on timers, `after` arms, `task.stopping()`, signals, and
  I/O do not count. Wakers uncount the woken thread before signaling it,
  cancellation bumps a generation counter, and a suspicion is reported
  only after 200 ms with no change; then the fault, a crash bundle (with
  the task tree), exit status 70. The report is the first wait cycle
  over "waits for" edges (mutex holder, awaited task, a scope's running
  tasks, Once runner), then the other blocked threads; without a cycle,
  every blocked thread. Each line: thread, spawn site, operation, call
  site, and the object (channel or mutex with where it was made, holder
  and since where).
- `xo dump <pid>` sends SIGQUIT; `RunMain` prints the task tree to stderr
  and writes it to `$XO_DUMP_DIR` or the temp dir as
  `xo-dump-<pid>.txt` (temporary file and rename), and the program goes
  on (Go's default SIGQUIT exit is replaced). `xo dump` polls for the
  file, prints it, removes it. Format: `xo dump: pid N, K tasks`, one
  indented line per task or scope with its state (`blocked in <op> at
  <loc> ...`, `in scope`, `waiting for its tasks`, `running`, `ready`,
  `(canceled)`), `end of xo dump`.
- Call sites: test worlds and the scheduler capture them with
  `runtime.Callers` at each block; plain real runs do not (about 200 ns
  per block, `gotchas/runtime-callers-per-block-cost.md`) and read them
  from the goroutine profile through a per thread `runtime/pprof` label
  when a dump or report needs them.
- Tests: goldens `test_explore` (args.txt `-explore 40`: a lost update
  race found at seed 2, pinned failing and passing seeds),
  `test_deadlock` (two task mutex cycle, a channel nobody sends on, a
  task awaiting itself), `deadlock_real` (real run, exit 70), changed
  `test_runner`; all three new ones in `TestGoldenRace` (which now passes
  `args.txt`); `TestRunSeedReproduces` (testdata/sched_run with
  XO_SEED), `TestTestSeedReproduces`; `cmd/xo` `TestDump`; std/rt
  `sched_test.go`.

## Record and replay, crash bundles (P2)

Last verified: 2026-10-08. Code: `std/rt/replay.go` (modes, recorder,
replayer, scheduler events, timers, verdicts), `std/rt/fxcodec.go` (value
codec), hooks in `caps.go`, `http.go`, `sched.go`, `main.go`, `stop.go`,
`chan.go`, `sync.go`, `log.go`, `core.go` (`Secret_new`);
`internal/build` `Info`; `cmd/xo/replay.go`. Spec: core 11.2, 11.3.

- Modes (read by `RunMain`, then removed from the environment so the
  program and its children do not see them): `XO_RECORD=<file>` (set by
  `xo run --record=<file>`, honored by any built binary) records a full
  trace, written when the program ends (normal end, error, fault,
  deadlock fault, `proc.exit`, third signal; a SIGKILL loses it; the
  trace is kept in memory until then). `XO_RING=N` keeps per request ring
  buffers of the last N effects (request 0 is main and its tasks), no
  scheduler, attached to crash bundles. `XO_REPLAY=<file>` (set by `xo
  replay`) replays a `.xotrace` or a `.xocrash`.
- Effects: every capability method runs its I/O through `fx(tk, op, io,
  args, real)`: recorded as op, encoded args, encoded result, Xo location;
  replayed from the per thread queue (per request for ring buffers)
  without doing the I/O. `io` effects (stdin reads, `proc.run`,
  `http.send`) repeat `leave`/`enter` in a replay so the interleaving is
  the same. Covered: `fs.*` (reads and writes; a replay never touches the
  filesystem), `net.listen`/`dial`, `clock.now`/`sleep`, `rand.*`,
  `env.*`, `stdio.read_line`/`read_all`, `proc.run`/`exit`, `http.send`,
  `http.serve`, log record times (`log.time`), the stop grace clock
  (`rt.mono`). Output (`stdio.print*`, `write`) is not recorded; a replay
  prints it again.
- Interleavings: a recording runs every task on the deterministic
  scheduler (seed `XO_SEED` or random, kept in the header) and logs every
  pick by thread id; `yield` always decides. Timers (`realAfter`, used by
  sleep, `after` arms, `with_timeout`, the grace timer), SIGINT/SIGTERM,
  accepted HTTP requests, and server ends are scheduler events (inbox,
  run at the next decision with no thread running). See
  `gotchas/record-replay-scheduler-decisions.md`.
- HTTP server: a recording reads the request, posts it as an event that
  creates the request thread, and logs the response (status, headers,
  body; streamed bodies teed). A replay's listener has no socket;
  `serve` waits for the trace's server end and requests are delivered
  from the trace; each response is compared with the recorded one.
- Trace format (JSON lines): header (`xotrace`, `mode` full or ring,
  `complete`, `seed`, `args`, `target`, `build`), `{"secret":k,"src":
  "env:NAME","at":...}`, effects `{"n","t","q","op","a","r","at"}`,
  requests `{"req","l","r","resp"}`, scheduler stream `{"s": "1 1*4 2 t3
  r1 g e1"}` (pick, run length, timer, request, signal, server end; `x`
  is a server end that cancels its requests), `{"end": {code, fault,
  fcode, loc, error, isolated, req}}`. Values: strings (non UTF-8 as
  `{"b64":...}`), numbers exact, struct fields without `F_` and omitted
  when zero, List as array, Map as [key, value] pairs, enums as
  `{"tag", "p": {"$": payload type, "v"}}`.
- Secrets: `Secret[T].new(v)` registers Str or Bytes values; every trace
  line has them replaced by `<redacted:k>` when written. A replay puts
  them back from `XO_REPLAY_SECRET_k` or the recorded env variable and
  fails before running (exit 3) naming both when neither is set.
- Replay verdicts on stderr, prefixed `xo replay:`: `identical run`
  or `reproduced the crash` with the outcome and counts; a divergence
  (other op or args, no more effects, unknown timer or server, a picked
  thread that never becomes ready for 3 s, different end or response)
  names the effect index, the task, and both locations, exit status 3
  (`rt.ReplayDiverged`). Bundles are prefixes: running past their end
  stops the replay; a reproduced isolated handler fault exits 0.
- Crash bundles (`writeCrashBundle`): written for a fault reaching main,
  an error returned from main (new 2026-10-08, `kind: "error"` with
  `frames`), a deadlock fault, and a fault isolated in a real server's
  request handler (at most 100 per process); stderr gets `crash bundle:
  <path>`. Fields: `kind`, fault, code, location, stack, detail or error,
  frames, `isolated`, `request`, `tasks`, `build` (id, dir, sources,
  options), `target`, `time`, `args`, and with a recording or ring
  buffers `trace` (one JSON line per element; ring bundles hold main's
  and the failing request's buffers, see
  `gotchas/ring-replay-other-requests.md`).
- `xo replay` reads the header, rebuilds from `build.dir` (or `--dir`)
  with the recorded options, warns when source hashes differ, and runs
  the binary with `XO_REPLAY`, the recorded args, and no stdin.
- Not recorded: `use go` functions that perform effects, the 5 s limit of
  shielded defers (real time), wall clock reads of the runtime outside
  capabilities. The LLVM backend has no record (`--record` is refused).
- Tests: `internal/gen/replay_test.go` (`TestReplayEffects`,
  `TestReplayInterleave`, `TestReplayServer`, `TestReplayDivergence`,
  each also on a -race build) on `internal/gen/testdata/replay/*`;
  `cmd/xo/replay_test.go` `TestRecordReplayCLI`.

## Benchmark tooling

Last verified: 2026-10-07

- `bench/` is its own Go module (`bench/go.mod`, module `xobench`) so the
  root module never builds benchmark code. Each Go solution under
  `bench/go/<arm>/<task>/` also has its own `go.mod`.
- Go analyzers for the strict arm are installed project locally in
  `.tools/bin` (staticcheck, errcheck, nilaway, exhaustive), installed with
  `GOBIN=$PWD/.tools/bin go install <pkg>@latest`. The built `xo` binary is
  also placed there (`go build -o .tools/bin/xo ./cmd/xo`).
- `bench/go/strict-check.sh <dir>`: vet, staticcheck, errcheck, nilaway,
  exhaustive, `go test -race`.
- `bench/perf/run.sh`: builds `bench/perf/xo` (Xo) and `bench/perf/go`
  (hand written Go, its own `go.mod`), runs each workload `REPS` times
  (default 5), and prints a Markdown table of the best times and the
  ratio; checksums must agree. Workloads: collection (`putget get push
  update iter copymut branch`), List writes (`setat stack fields grid
  insrem copyset`, added 2026-10-08, Go slices on the Go side; an xo
  binary from before 2026-10-08 does not finish `setat` or `stack`), and
  generic (`sortint sortpt hof maxby iface`, idiomatic Go generics on the
  Go side). `MONO=1` builds the
  "Xo" column with `--no-mono` (when the binary has the flag; an older
  binary's default is that lowering) and adds an "Xo --mono" column (the
  default lowering), `XOFLAGS` passes flags to the first Xo build,
  `XO=<path>` uses another xo binary (for before and after runs),
  `WORKLOADS="..."` selects workloads, `PERF_OUT=<dir>` keeps the
  binaries. Timing drifts on the laptop host; interleave before and after
  runs (`gotchas/bench-host-timing-drift.md`). Last verified: 2026-10-08.
- `bench/perf/http/run.sh` (2026-10-08): HTTP load benchmark of task 01.
  `go/` and `xo/` are copies of `bench/go/plain/01-crud-api` and
  `bench/xo/v0.4/01-crud-api` that listen on 127.0.0.1 only (since
  2026-10-08 `xo/` reads the store with `Mutex.with_read`, so the
  `xo-base` configs need an xo binary that has it; for older binaries
  build a copy of the source with `with` instead); `loadgen/`
  (own module) runs `CONC` keep alive workers (default 32) through a
  create, get, patch, get, delete cycle (and a list every eighth cycle)
  for `WARMUP` + `DURATION` (2s + 10s). Each server starts fresh with
  `GODEBUG=gctrace=1` and is stopped with `kill` of its own PID. Prints
  raw lines and a Markdown table of medians over `REPS` (3) interleaved
  runs: req/s, p50/p99/p99.9, GC count, total STW pause, peak heap, GC
  CPU percent, peak RSS, GCs per 10k requests. `CONFIGS` (default `go xo
  xo-arenas`; `xo-base` builds with the binary in `XO_BASE` in its own
  build directory, `xo-base-arenas` the same with `--arenas`), `XO`, `PERF_OUT`, `PRELOAD`, `PORT` (18080). The load
  is closed loop (`gotchas/closed-loop-latency-confound.md`). Results:
  `bench/results/perf-arenas-2026-10-08.md`,
  `bench/results/perf-throughput-2026-10-08.md`.
- Profiling a built Xo program (2026-10-08): `XO_CPUPROFILE=<file>` writes
  a CPU profile of the whole run and `XO_BLOCKPROFILE=<file>` a block
  profile (where goroutines waited: locks, channels, select) when
  `RunMain` returns, so stop a server with `kill <its PID>` (SIGTERM, a
  clean shutdown), not SIGKILL. Read with `go tool pprof -top -cum
  <binary> <file>`. Last verified: 2026-10-08.
- `bench/perf/llvm/run.sh` (2026-10-08): the LLVM backend prototype
  against the Go backend and hand written Go on compute workloads in the
  LLVM subset (`fib sieve nbody sort interp collatz strings`; `strings`
  added 2026-10-08 for allocation churn). `RSS=1` adds a peak memory
  table (`/usr/bin/time -l`). Each Xo workload
  is its own program (`xo/<name>/main.xo`, the subset has no arguments or
  clock); `go/` (own module) is one binary `llperf <name>`, plus
  `llperf interp_tuned` (a tuned interpreter, not part of the script).
  Wall clock time per process, best of `REPS` (5); a run longer than
  `TIMEOUT` seconds (60) is stopped and shown as `>60000`, needed with xo
  binaries from before 2026-10-08, whose list element writes were O(n)
  (`gotchas/go-backend-list-set-quadratic.md`). Checksums must agree.
  Needs python3 for timing. `XO`, `PERF_OUT`, `WORKLOADS` as in
  `bench/perf/run.sh`. Results: `bench/results/llvm-prototype-2026-10-08.md`,
  `bench/results/llvm-rc-2026-10-08.md` (reference counting).
- `bench/perf/llvm/native.sh` (2026-10-08): the native arm64 backend
  against the LLVM backend and hand written Go on the same workloads.
  Binaries run interleaved (Go, LLVM, arm64) `REPS` times (5) under
  `/usr/bin/time -l`; the table has best wall time (`STAT=median`: the
  median, added 2026-10-08), the two ratios, and peak RSS of all three.
  Checksums must agree. `XO`, `PERF_OUT`, `WORKLOADS` as above. Check the
  load average first (`gotchas/bench-host-timing-drift.md`).
  `/usr/bin/time -l` also prints instructions retired and cycles, which
  separate "more instructions" from "slower instructions". Results:
  `bench/results/arm64-native-2026-10-08.md`,
  `bench/results/arm64-perf-2026-10-08.md`.
- Instruction level profiles on macOS (Xcode 16 `xctrace`, no root;
  Last verified: 2026-10-08). Make the program run for seconds first
  (more rounds in a copy of the bench program), then
  `xctrace record --template 'CPU Profiler' --output p.trace --launch -- ./bin`
  (a sample every 1M cycles, user and kernel) and
  `xctrace export --input p.trace --xpath '/trace-toc/run[@number="1"]/data/table[@schema="cpu-profile"]'`:
  each row has a `cycle-weight` and a backtrace whose first frame is the
  sampled PC; its `binary` element (often a `ref` to an earlier id) has
  the `load-addr`, so `pc - load-addr + 0x100000000` is the address in
  `otool -tvV ./bin`. Samples skid to the instruction after a stall. The
  `CPU Counters` template (default "bottleneck" mode) gives, in table
  `CounterMetricAggregatedForProcess`, one row per 1 ms of four values
  in hundredths of a percent: useful, instruction processing,
  instruction delivery, and discarded (that order). Useful is
  instruction based: compare it per instruction of work, not across
  programs that retire different counts. xctrace sometimes dies with
  `Trace/BPT trap` under load; rerun. Neither is in the repository's
  scripts; the parsing is a few lines of Python.
- `bench/planted/run_detection.py [task...]`: runs planted bug variants
  through the plain and strict loops, writes `bench/planted/results.jsonl`.

## Package layout (P1, implemented parts)

```
cmd/xo/                 CLI entry point; `xo parse`, `xo check`, `xo build`, `xo run`, `xo test`, `xo bind`,
                        `xo fmt`, `xo vet`, `xo query`, `xo doc`, `xo explain` (tools.go), `xo dump` (dump.go),
                        `xo mcp`, `xo serve` (agent.go)
                        `xo fmt`, `xo vet`, `xo query`, `xo doc`, `xo explain` (tools.go), `xo dump` (dump.go), `xo fix` (fix.go)
internal/token/         token kinds, keywords, Pos and Span (file, line, col, offset)
internal/lexer/         tokens, newline termination (core 1.3), string interpolation parts,
                        doc comments
internal/ast/           syntax tree for the whole 0.4 grammar; JSON dump (ast.ToJSON/Dump)
internal/parser/        recursive descent parser with error recovery; SignatureMode
internal/diag/          diagnostics (core 10.2 JSON), codes.go is the registry of all codes
internal/check/         name resolution and type checking (see below)
internal/format/        canonical printer for `xo fmt`; format.Decl prints signatures
internal/vet/           `xo vet` advisory checks (XO0207, XO0208, XO0703, XO0901, XO1103)
internal/fix/           `xo fix`: the check fix loop and the migration rewriter registry
internal/query/         `xo query` over a check run; Surface is shared with internal/doc
internal/edit/          `xo edit`: rename, add-field, change-sig through resolved symbols
internal/mcp/           `xo mcp`: MCP (JSON-RPC 2.0 over stdio) tools check, query, explain, fmt, test, build
internal/serve/         `xo serve`: unix socket server, cache of checked sessions keyed by file mtimes
internal/doc/           `xo doc` Markdown and JSON documentation
internal/gen/           Go code generation with //line directives (see "Code generation")
internal/build/         check, generate, write the build directory, `go build` per target;
                        llvm.go: the LLVM backend build (IR + std/llrt runtime -> clang)
                        arm64.go: the native backend build (assembly + std/llrt runtime -> cc)
internal/llvm/          P3 prototype: LLVM IR from internal/ssa (see "LLVM backend (P3 prototype)")
internal/arm64/         native AArch64 backend from internal/ssa (decision 0049, see "arm64 backend (native)")
internal/ssa/           shared typed SSA IR for native backends (decision 0045): builder from the
                        checked AST, verifier, printer; goldens in testdata/ (see "SSA IR")
internal/gobind/        `use go` bindings: go list + go/importer + go/types -> Xo declaration
                        file and Go adapter (decision 0041 prototype, see "Go interop")
std/                    std declaration files (prelude.xo, go.xo, http.xo, json.xo, log.xo,
                        path.xo, sql.xo, task.xo, testing.xo, time.xo) embedded by std/embed.go
std/rt/                 package rt, the runtime linked into every program (Go, embedded
                        by std/embed.go as std.Runtime and copied into each build)
std/llrt/               xo_rt.c, the C runtime of the LLVM and arm64 backends (embedded as std.LLRuntime);
                        testdata/rc_threads.c, its thread stress test (not embedded)
```

`std/*.xo` are bodiless signature listings of `spec/std.md` (draft 0.4),
parsed in `parser.SignatureMode`. The encoding of std.md pseudo syntax
(`[var]` receivers, `Int.parse` statics, implicit receiver type parameters,
opaque handles, `@moves`) is documented at the top of `std/prelude.xo`. The
prelude imports `std/task` for `task.Group` (`os.tasks`, `s.group()`), so
`std/task` is always loaded. Last verified: 2026-10-07.

`internal/check` (package `check`, entry point `check.Check(dirs)`):

- `load.go`: module discovery, xo.mod roots, import resolution (XO0602,
  XO0604), std loading.
- `decl.go`: top level objects, type declarations, signatures, methods,
  effect clauses, type expressions (XO0101, XO0603, XO0606, XO0702).
- `types.go`, `infer.go`: types, substitution, unification with a trail,
  assignability with the three implicit conversions (core 2.10), interface
  satisfaction (structural, plus builtin rules for Eq, Hash, Ord, Json,
  Display, Debug), error conversion at `?`.
- `expr.go`, `call.go`, `stmt.go`: bidirectional checking of bodies. The
  expected type of a value is a type, a `retWant` (return position of
  `-> T ! E`), or `discard` (a value whose Result must be handled, XO0201).
- `pattern.go`: pattern typing and exhaustiveness (usefulness algorithm with
  witnesses, XO0301 to XO0304).
- `effects.go`: effect sets with labels, effect parameters, effect inference
  variables, and references to the inferred effects of non pub functions;
  fixpoint and XO0701 obligations with call chains (`chainSteps`, shared
  with `query.go`).
- `query.go` (2026-10-08): accessors for the tools: `Inspect`,
  `Info.Performed` (effects a body performs with call chains),
  `Info.Funcs`. `File.Src` keeps each file's source.
- `flow.go`: moves of `Buf`/`StrBuilder` (XO0510) and lost updates (XO0520:
  copies out of a collection, including `.or_err(...)?`, and locals mutated
  after being stored into a collection), run after typing.
- `groups.go`: task groups (core 7.1): failure of group tasks (XO0801) and
  values holding a group escaping their scope (XO0802).
- Effect argument elision (core 6.2) is in `decl.go` (`resolveArgsFor`,
  `elidedEffect`): implicit effect variables in parameter and receiver
  types, implicit effect parameters on struct and enum fields, inference in
  local bindings and in result types.

Checker tests in `internal/check`: `rules_test.go` (table driven positive and
negative cases per rule), `planted_test.go` (the Xo side of
`bench/planted/`), `golden_test.go` with `testdata/golden/<case>/want.json`,
`rules04_test.go` (rules new in spec 0.4: XO0206, `is`, tuple fields, local
`const`, widened XO0520, task groups, effect elision, std 0.4 surface),
`corpus_test.go` (bench/paper/v0.3 as data under the current spec, and spec
Appendix A), `mutation_test.go` (opt in), `coverage_test.go` (every emitted
code has a positive case).

Parser notes for spec 0.4 (last verified 2026-10-07): `x is Pattern` is an
`ast.IsExpr` parsed at the precedence of `==` when the identifier `is`
follows an operand; a tuple field `t.0` is an `ast.SelectorExpr` whose `Sel`
is the decimal index (the lexer does not read a fraction after `.`); a local
`const` is an `ast.ConstDecl` statement; `&= |= ^= <<= >>=` are rejected; a
result type may start on the line after the parameters (`->` continuation,
std.md 7).

Parser tests live in `internal/parser`:

- `parser_test.go`: table driven expression, statement, pattern, declaration,
  doc comment, span, error recovery, and fix tests; a no panic test on
  truncated input; `FuzzParse`.
- `spec_test.go`: extracts every fenced block from `spec/core.md` and parses it
  as a file, then as a function body, then in signature mode. Non Xo blocks are
  in an explicit skip table, the 1.6 literal listing is lexed only, and spec
  bugs are listed in `specKnownFailures` (empty since spec 0.4).
  `spec/std.md` blocks are parsed in signature mode after rewriting `[var]`
  to `var`; pseudo signatures such as `Int.parse(s: Str) -> Int?` are dropped
  and logged.
- `corpus_test.go`: every `.xo` file under `bench/paper/v0.2/` must parse;
  `bench/paper/v0.1/` is parsed for information only.
- `sexpr_test.go`: compact S-expression printer used by the table tests.

## Build pipeline (P1)

`.xo` sources -> lexer -> parser -> checker -> Go source (internal/gen, with
`//line` directives) -> build directory with the runtime -> `go build` with
`GOOS`/`GOARCH` per target and `CGO_ENABLED=0` -> static binaries. All stages
are implemented. Last verified: 2026-10-07.

With `--backend=llvm` (2026-10-08): checker -> SSA with reference
counting (internal/ssa) -> LLVM IR (internal/llvm) ->
`<build dir>/llvm/prog.ll` plus `xo_rt.c` -> `clang -O2 -flto` -> one host
binary. The build directory keeps `prog.ll` for inspection.

With `--backend=arm64` (2026-10-08): checker -> SSA (internal/ssa,
`InsertRC`, verified) -> assembly (internal/arm64) ->
`<build dir>/arm64/prog-<os>.s` plus `xo_rt.c` -> `cc -O2` -> one binary
for darwin/arm64 or linux/arm64 when that is the host; another arm64
target is written as assembly only (`build.Result.Asm`).

## Code generation

Last verified: 2026-10-08. Rules and choices; details in
`bench/results/gen-run-v0.4.md`.

- One Go package per Xo module (`xoprog/m/x_<module path>`), one file
  `xo.go`. Std modules map onto package `rt`. Go names: types keep their
  Xo name (std types are `<Module>_<Name>` in rt, `Http_Request`), payload
  types `<Enum>_<Variant>`, functions `F_<name>`, constants `C_<NAME>`,
  methods `M_<name>`, fields `F_<name>`, locals `v_<name>` (numbered when
  reused), temporaries `x_<kind><n>`, type parameters `tp_<Name>`.
- Every function, method, and closure takes `tk *rt.Task` first.
- Types: Int is int64 (sized types map to Go sized types), Float float64,
  Str string, Bytes `rt.Bytes` (a string), Rune `rt.Rune`, Duration and
  Time `rt.Duration`/`rt.Time` (int64 nanoseconds), `()` `rt.Unit`, tuples
  `rt.TupleN`, `T?` `rt.Option[T]` (a value: Some flag and V), `Result`
  `rt.Result[T, E]` (with the error frame trace), structs Go structs,
  enums `struct{ Tag int; P any }` with a payload struct per variant (so
  recursive enums work), interfaces Go interfaces with `M_` methods,
  `Display`/`Error`/`Debug` `rt.IDisplay`/`rt.IError`/`rt.IDebug`,
  `Chan`/`Mutex`/`Atomic` pointers to rt handles, List/Map/Set rt
  persistent collections.
- A function value of type `fn(A) -> R ! E` is `func(*rt.Task, A)
  rt.Result[R, E]`; one that cannot fail uses E = `rt.Never`, so the form
  never depends on whether a generic error parameter was instantiated with
  Never. Declared functions return R, `rt.Result[R, E]`, or nothing for
  `()`; a call whose declared error became Never takes `.V`. Declared
  functions used as values are wrapped in closures; subsumption (a function
  that cannot fail where a failing one is expected, a converting error)
  is an adapter closure.
- Generics are Go generics (constraints are `any`; Eq, Hash, Ord, Display
  go through rt functions that switch on the dynamic type or call the
  generated `XoEq`/`XoHash`/`XoCmp`/`XoDebug`/`M_display` methods). A
  method is a Go method unless it has its own type parameters, its
  receiver is a scalar prelude type, its receiver restricts the type's
  arguments (`List[Str].join`), or its signature instantiates its own
  receiver type with other arguments (Go instantiation cycle); then it is a
  top level function `<Type>_M_<name>` (user) or `<Type>_<name>` (rt).
- Expressions that need statements (blocks, if, match, `?`, `??`, scope,
  select, `with`) are hoisted; operands before one that needs statements
  are spilled into temporaries to keep left to right evaluation, except
  places (`var` receivers, `&` arguments). Blocks deliver their value to a
  destination: discard, assign, or return (so arms of a tail if/match
  return directly, core 2.6).
- A block that contains `defer` runs as `func() int { ... }()`; Go's
  defer gives block scoping, `tk.Shielded` the cancellation shield (core
  3.5, 7.2). `return`, `break`, `continue`, and `?` cross it as control
  codes dispatched after the call. `scope` is the same with
  `rt.Scope.Close` deferred.
- Integer arithmetic, conversions, indexing, and slicing call rt functions
  that fault with the values and the `file:line:col` of the operation;
  rt faults without a position use the innermost `.xo` frame of the stack.
  Int (int64) operations (2026-10-08): before each body is generated, a
  range analysis (internal/gen/ranges.go: intervals per binding through
  constants, `let`/`var`, range loops, comparisons, `%`, masks, `len()`,
  sized integer widening, loops iterated to a fixed point with widening,
  plus symbolic `i <= len(xs) - 1` bounds for lists under parameters and
  `let`/`for` bindings) marks operations that cannot fault; they are
  plain Go, and `xs[i]` proven in range is `List.AtU`. Other operations
  with a constant operand use one comparison forms (`rt.AddIUp`,
  `rt.SubIDown`, `rt.MulIK`/`MulKI`, plain `/` and `%` by a constant other
  than 0 and -1); the rest call `rt.AddI..RemI` and `List.AtM` (typed,
  inlinable, fault by `panic(&arithFault{...})`, converted to a
  `*FaultErr` with the operation's stack at the first recover), or under
  `--no-mono` the generic `rt.AddS` family. `TestRangeElision`,
  `TestRangeDifferential` (analysis on and off must print the same values
  and faults), and golden `test_range_checks` cover it.
- `for i in a..b` and `a..=b` over a range literal are counted Go loops
  (bounds evaluated once); other Range values iterate `.All()`
  (2026-10-08, `gotchas/range-over-func-loop-cost.md`).
- A struct field whose type contains the struct by value (through Option,
  Result, tuples, other structs) is stored in `rt.Box[T]`.
- Assignment resolves the place first (index operands left to right), then
  writes. Through List elements and fields it writes in place: each list
  on the path is made the unique owner of its array (`if !p.Mine() { p =
  p.Unshared() }`), then `p.PtrM(i)` or `p.PutM(i, v)`; a compound
  assignment uses one address for the read and the write. Through a Map
  value or a box it reads, marks, modifies, and writes back
  (`m[k].count += 1`). A std `var` method on an element (`xs[i].push(x)`)
  gets the element's address; a user `var` method gets a marked copy
  that is written back. The value is computed first when it could write
  the place's root (2026-10-08).
- List ownership (2026-10-08, internal/gen/own.go, rationale and numbers
  in `bench/results/perf-list-2026-10-08.md`): a read of a binding,
  field, or element whose type holds lists is wrapped in an own mark
  (`(x).Own()`, or a function literal calling generated `XoMark` methods
  for structs, tuples, Options, Results; `rt.Own` for type parameters)
  unless the reading expression borrows it: field and element bases,
  receivers of read only List methods, comparison, `+`, and
  interpolation operands, loop subjects the body does not write, by value
  arguments of user calls without `var` parameters (each checked with
  `mutates`; `var` parameters may alias). Std call results holding lists
  are marked except fresh producers (`freshStd`); closures mark what they
  capture; interface conversions mark. Structs and newtypes whose fields
  hold lists get `func (v *T) XoMark()`. `push` on a binding or field
  path is `if p.CanPush() { p = p.PushFast(x) } else { p = p.PushSlow(x)
  }`. A field read of an element is `xs.RefM(i, at).F_f` (no copy of the
  element) when the statement cannot write the list. Rules for changes:
  `gotchas/list-own-marks.md`. Tests: golden `list_value_semantics`
  (also under `-race`), `TestListModel`.
- Monomorphization (internal/gen/mono.go; added 2026-10-08 as `--mono`,
  the default since 2026-10-08; `--no-mono` on `xo build`, `run`, `test`,
  `gen.Options.NoMono`, `build.Options.NoMono` turn it off, `--mono` is
  still accepted): calls of user generic functions and methods with
  concrete type arguments call specialized copies `<name>__<types>` generated into the
  calling package (methods of generic types as top level copies, receiver
  first); `==`/`<` on a type parameter in a copy lower as for the concrete
  type, with typed `XoEqT`/`XoCmpT` methods generated for non generic
  structs and newtypes; constraint method calls call the concrete method;
  `List.map/filter/fold/any/all/find` with an inlinable closure literal
  become loops with the body inlined; a non capturing literal passed to a
  user generic function gets a `_lit` copy that inlines it. Fallback to
  the Go generic function: non concrete type arguments, copy chains
  deeper than 8, more than 64 copies of one function per package. Go
  forms are decided on the generic types, so copies keep the generic
  signature form. The build directory hash includes the lowering.
  `TestGolden` (default lowering, Go frame lists dropped) and
  `TestGoldenNoMono` (exact) run every golden program both ways. Numbers:
  `bench/results/perf-mono-2026-10-08.md`.
- Request scoped arenas (2026-10-08, internal/gen/arena.go, `--arenas`
  on `xo build`, `run`, `test`; `gen.Options.Arenas`,
  `build.Options.Arenas`; off by default, and off generates exactly the
  code it did before): the generated main calls `rt.EnableArenas()`. A
  closure literal passed to `r.handle` on a local router whose request
  parameter is used only through `json`, `param`, `param_int`, `query`,
  `header`, and the Str fields (never in a nested closure) is followed by
  `rt.Http_Router_arenaLast(&r)`. `http.json`/`http.text` calls whose
  value is the handler literal's result (tail of the body through
  if/match/block tails, or a `return`) become `rt.Http_json_arena` /
  `rt.Http_text_arena`. `m.values()`, `m.keys()`, `m.entries()`,
  `s.to_list()` that are only the receiver of a List consumer that keeps
  nothing (`arenaConsumers`) or the iterable of a `for` become
  `rt.Map_values_arena` and friends. The build directory hash includes
  the flag. Tests: `TestArenaEscapeAnalysis`, `TestGoldenArenas` (every
  golden with `--arenas`), `TestGoldenRaceArenas`, golden `arena_escape`.
- Interface conversions (2026-10-08, internal/gen/adapt.go): a value whose
  Go methods differ from the interface's (a method that cannot fail where
  the interface's can) is wrapped in a generated adapter; inside a
  specialized copy the concrete type is known; an interface value is
  wrapped around the source interface; generic Go code converts with
  `rt.Iface`, which consults adapters that package main registers with
  `rt.RegAdapter` for every concrete user type named in generated code and
  every interface generic code converts a type parameter to.
- The std structs, enums, and interfaces are generated into
  `std/rt/std_gen.go` by `gen.StdTypes`; `TestStdTypes` fails when it is
  stale. Opaque std handles and every std function are hand written in rt.
- std/sql (2026-10-08, std.md 10): `std/sql.xo` declares Db, Tx, Row, SqlValue,
  SqlErr; `std/rt/sql.go` implements them over a Go `SqlBackend` interface
  (values nil, int64, string, float64, bool, Time; errors `*SqlError` by kind) with
  a cancelable pool (`lock`, `max_open` permits, plus one writer permit for single
  writer backends). `std/rt/sqlmem.go` is the in memory driver (small SQL parser
  and evaluator; tables are immutable and swapped on write, a transaction is a
  copy of the table map). Real drivers: implement `rt.SqlOpener`, call
  `rt.RegisterSqlDriver` (to be bound through `use go`; none yet). Goldens:
  `sql_basic`, `sql_tx`, `test_sql`, `sql_decode`. Not recorded by record and replay.
  `derive SqlRow` (2026-10-08, decision 0061): `internal/gen/sqlrow.go` emits
  `(*T).XoSqlRowDec`, one `rt.SqlRowField` call per field
  (`std/rt/sqlrow.go`, a type switch over the column types);
  `Sql_Row_decode[T]`, `Sql_Db_query_as[T]`, `Sql_Db_query_one_as[T]` and
  the `Sql_Tx_` forms assert `sqlRowDecoder` on `*T`. `RETURNING` in
  `sqlmem.go`: writes return a `memWrite` (count, last id, projected rows);
  `memBackend.Query` and `memTx.Query` run on a copy of the table map and
  publish it on success. Checker golden: `derive_sqlrow`.
- std/http request values and scoped middleware (2026-10-08, decision
  0055): `Http_Key[T]`, `Http_Request_with_value`/`_value` (a persistent
  list in the hidden `XValues` field of `Http_Request`, listed in
  `rtExtraFields` in internal/gen/json.go), `Http_Router_wrap[E]` (generic
  now; was the Go method `M_wrap`), `M_mount` (flattens the sub router's
  routes, each behind `chain(sub.mws, h)`), `Listener.M_addr` (the
  `addr` field, also in the recorded `net.listen` result). Golden:
  `http_scoped`.
- std/crypto (2026-10-08, decision 0056, std.md 11): `std/crypto.xo`,
  `std/rt/crypto.go` on `crypto/sha256`, `crypto/hmac`, `crypto/subtle`,
  `crypto/pbkdf2` (Go 1.24+); random functions call the `Rand`
  capability's `M_bytes`. Goldens `crypto_basic` (known vectors),
  `test_crypto` (seeded rand); unit tests `TestCrypto*` in std/rt.
  Argon2id (2026-10-08, decision 0059): `Crypto_hash_password_argon2id`
  and the `$argon2id$` branch of `Crypto_verify_password` call
  `argon2Key` in the vendored `xcrypto_argon2.go`; tests in
  `std/rt/xcrypto_test.go` (`TestArgon2RFC9106`, `TestBlake2bRFC7693`,
  `TestArgon2idMatchesXCrypto`, `TestCryptoArgon2idHash`) and
  `BenchmarkArgon2idDefault` (about 19 ms per hash on Apple M, 2026-10-08).
- Test log capture (2026-10-08, decision 0057): `runOneTest` gives each
  test world `newTestLogSink()` (std/rt/log.go); `logRecord` appends a
  `Log_Record` and, with no configured output, the text line to the
  capture instead of stderr; `RunTests` prints the lines under `--- FAIL`
  (and `--- PASS` with `-v`), `--json` adds `log`. `testing.logs()` is
  `rt.Testing_logs`. Golden: `test_logs`.
- Fake network listeners (2026-10-08): `memListener.served` is fired when
  `serve` starts; `Testing_FakeNet.dispatch` waits on it (wait kind
  `waitAccept`) so a request sent before `serve` queues like a backlog
  (gotcha `fake-listener-before-serve.md`).

## SSA IR

Last verified: 2026-10-08. Decisions 0045, 0046, 0048. `ssa.Build(res)`
lowers the functions reachable from `main` to SSA with block parameters
(generic functions monomorphized per type arguments, closures as
`Indirect` functions with a counted environment, recursive fields as
`Box[T]`, failing functions returning their Result); `Module.Verify()` checks structure,
dominance, and types; `Module.String()` prints it. `ssa.BuildOpts` takes
a `LineRoot` for fault positions. `Module.InsertRC()` (decision 0046,
`rc.go`) adds reference counting: borrow inference (`Func.Borrowed`,
printed `borrow v0`), retain and release per heap leaf, cancellation,
and `reuse` before list updates not proven unique; afterwards
`Module.RC` is set and `Verify` also checks RC balance on every path
(`verify_rc.go`). Tests: `go test ./internal/ssa`
(testdata/golden/<name>/main.xo with want.ssa and want.rc.ssa;
testdata/bench/<name>.ssa and .rc.ssa for bench/perf/llvm/xo; verifier
rejections; `TestVerifyRCMutations` deletes each inserted retain and
release and requires a rejection). Rewrite goldens with
`go test ./internal/ssa -update`. internal/llvm lowers from it
(below).

## LLVM backend (P3 prototype)

Last verified: 2026-10-08. Experiment for decision 0040 ("own backend");
results and design: `bench/results/llvm-prototype-2026-10-08.md` (the
first, AST based prototype, since deleted) and
`bench/results/llvm-rc-2026-10-08.md` (SSA with reference counting).

- `xo build --backend=llvm` and `xo run --backend=llvm`; host only,
  programs only (not `xo test`), no `--race`, `--arenas`, `--holes`,
  `--targets` other than the host. `build.Options.Backend`.
- One path: `llvm.Generate` builds SSA (`ssa.BuildOpts` with the line
  root), verifies, runs `InsertRC`, verifies again (RC balance per
  path), and lowers with `GenerateSSA` (`internal/llvm/fromssa.go`;
  decisions 0046 and 0048): SSA values are LLVM values, block parameters
  phis, `retain`/`release` call generated `xo.dup.<T>`/`xo.drop.<T>`
  helpers, `reuse` is a count check with a cold copy (`xo_rl_copy`), list
  updates write in place, `==` on aggregates calls generated `xo.eq.<T>`
  helpers. A construct outside the SSA builder's subset is a build error
  naming it and its position ("the SSA builder does not support ...").
  The AST path (bump allocator, never freed) was deleted on 2026-10-08.
- Layout: Int i64, Bool i1, Float double, `()` and capabilities `{}`, Str
  `{ptr obj, i64 len}` with obj `{i64 rc, i64 cap, bytes}` (literals are
  constants with the immortal count INT64_MIN, never written), List a
  pointer to `{i64 rc, i64 len, i64
  cap}` plus elements, structs and tuples LLVM structs, enums `{i64 tag,
  [N x i64]}` (payloads through a stack slot), function values `{ptr
  code, ptr env}` with env null or `{i64 rc, ptr drop, ptr share,
  captures...}`,
  `Box[T]` a pointer to `{i64 rc, T}`. A failing function returns its
  Result enum. Checked Int operations use `llvm.s*.with.overflow` and cold
  fault calls; loop counters are unchecked (the range analysis of
  internal/gen is not used by this backend).
- Subset: what `ssa.Build` lowers (its doc comment lists it): Int, Bool,
  Float, Str, printing, structs, enums, Option, Result, tuples, `match`,
  `if let`, `is`, the usual patterns (not list patterns), loops, `for`
  over ranges, lists, and `xs.indexed()`, functions, methods, `var`
  parameters, recursion, generics, closures, function values, recursive
  types, failing functions with `?` and `from` conversion, `??`,
  `or_fault`, `fault`, `const`, `==` on every type, the List methods
  push, pop, set, get, first, last, len, is_empty, and the Int, Float,
  and Str methods the bench programs use.
- Counts (decision 0058): positive is thread local (plain updates),
  negative is shared (atomic: `atomicrmw`, counts read with `load atomic
  unordered`), INT64_MIN immortal. `OpShare` calls generated
  `xo.share.<T>` helpers that mark a value's heap graph shared. Nothing
  emits `OpShare` yet except `build.Options.ShareProbes` /
  `llvm.Options.ShareProbes` (tests: every heap value marked shared as it
  is defined; builds go to `<build dir>/llvm-share`).
- Runtime `std/llrt/xo_rt.c`: size class allocator (16 byte classes to
  1 KiB, powers of two to 256 KiB, malloc above; free lists; 4 MiB
  chunks) with allocation counters (`XO_LEAKCHECK=1` prints live and
  total at exit); per thread heaps after `xo_rt_threads_begin()` (one
  pointer load while single threaded), `xo_rc_inc`/`xo_rc_dec`/
  `xo_rc_claim`/`xo_rc_mark` (the count semantics generated code inlines),
  a locked output buffer when threaded; `-DXO_RT_TEST_THREADS=N` runs the
  program on N threads at once, `-DXO_RT_NO_MAIN` leaves out `main` (the
  C tests in `std/llrt/testdata/` include the runtime); reference counted strings (`xo_rs_*`, append in place when
  unique) and lists (`xo_rl_*`); `xo_rc_defer`, a worklist that frees
  closure environments and boxes whose count reached zero without
  recursing on long chains; buffered stdout; fault messages identical to
  std/rt's and exit status 70; Go style float formatting (shortest round
  trip `'g'`, `.0` appended); `main` runs the program on a thread with a
  1 GiB stack. The C interface passes only scalars and pointers.
- Fault locations are `file:line:col` strings in the binary (no DWARF).
  Fault output has no Go frame lines and no crash bundle; the golden test
  compares fault messages after dropping those lines.
- Tests: `internal/llvm/llvm_test.go`. TestGolden builds each program in
  `testdata/golden/` with the Go backend and the LLVM backend and
  requires equal exit status, stdout, and fault messages, also matching
  `want.txt`; programs that exit normally must free every allocation
  (`XO_LEAKCHECK=1`). The `rc_*` programs are copies of SSA goldens
  (`internal/ssa/testdata/golden`) that stress ownership. TestLeakCheck
  does the same for the bench programs. TestGoldenShared runs every
  golden with share probes. `threads_test.go`: TestRuntimeThreads
  (`std/llrt/testdata/rc_threads.c` under ThreadSanitizer, then `-O2`
  with more iterations) and TestGoldenThreads (goldens that exit 0, IR
  with probes, four threads at once, ThreadSanitizer except the deep
  recursion ones, `gotchas/tsan-deep-recursion.md`); both skip when
  clang has no ThreadSanitizer. TestUnsupported checks the error
  for constructs outside the subset (`testdata/unsupported/`).
  `cmd/xo/llvm_test.go` drives the CLI.

## arm64 backend (native)

Last verified: 2026-10-08. Decisions 0049, 0051, and 0054; results
`bench/results/arm64-native-2026-10-08.md` and
`bench/results/arm64-perf-2026-10-08.md`.

- `xo build --backend=arm64` and `xo run --backend=arm64`; programs only,
  no `--race`, `--arenas`, `--holes`; targets darwin/arm64 and
  linux/arm64. `build.Arm64Asm(res, root, goos)` returns the assembly
  (it retries without inlining if the inlined module does not verify,
  `arm64.InlineError`). Every op the SSA builder and InsertRC emit is
  lowered (decision 0050: closures, boxes, structural equality); an op
  outside that is an error, no fallback.
- Pipeline per function (`internal/arm64`): `isel.go` (SSA to MIR over
  virtual registers with assembly templates; values flattened into words,
  `layout.go`), `opt.go` (memos inherited from dominators, list length
  registers, dead code, constant tag and inlined predicate threading,
  branch bounds, constant hoisting out of loops, branchless select of a
  compare picked by another compare, flag reuse), `jumptab.go` (`match`
  compare chains laid out as runs of tests, jump tables from 8 values,
  decision 0051; `XO_ARM64_JT_MIN=n` overrides the threshold for
  measurement, 0 disables tables), `rc.go` also cancels a payload retain
  against the release of its enum (`ssa.CancelledRelease`, which the
  LLVM backend uses too), `regalloc.go` (liveness, live ranges with holes, move
  coalescing, greedy first fit by spill weight, spill slots),
  `emit.go` (frames, parallel moves, fault stubs with operand recovery),
  `rc.go` (dup and drop helpers in assembly), `lsr.go` (decision 0054:
  loops found on the SSA, pointer induction variables for list element
  accesses, induction variable bounds, `xs[i + c]` from xs[i]'s pointer,
  rotation of a back edge at a bounds check (`_c` labels), integer `ldp`
  pairs as a text peephole, latch layout around inner loops),
  `ranges.go` (intervals of Int values: unchecked multiplies, `and`/`asr`
  for `%`/`/` by 2^k of non negative values), `closures.go` (function
  values, environments, `blr x0`), `boxes.go` (Box cells), `eq.go`
  (`==` helpers), `inline.go` (small leaf functions, SSA to SSA, before
  lowering).
- ABI: AAPCS64 (x0 to x7, d0 to d7, 8 byte stack slots for more Xo to Xo
  arguments; result words past 8 of a class are stored by the callee in
  the caller's stack argument area). Scratch x15 to x17 and
  d29 to d31; x18 never used. Frame record kept in non leaf functions,
  callee saved pairs stored high register first
  (`gotchas/arm64-stp-pair-order.md`). Darwin: `_` prefix, Mach-O
  sections, `@PAGE`/`@PAGEOFF`; Linux: ELF, `:lo12:`, `.note.GNU-stack`.
- Layout differs from the LLVM backend's: an enum payload has one 8 byte
  word per field word (a Bool field takes a word); list elements are
  8 bytes per word except `List[Bool]` (1 byte). The runtime sees only
  element sizes and callbacks.
- Counts (decision 0058): retains inline `tbnz #63` to a cold atomic
  block, releases `cmp #1; b.le` to a cold path that frees at one,
  ignores the immortal count (bit 62 clear) and otherwise drops a shared
  reference atomically: LSE `stadd`/`ldaddal` on Darwin, `ldxr`/`stxr`
  and `ldaxr`/`stlxr` loops on Linux (scratch x17 and w15 in inline
  templates). Environments are `{rc, drop, share, captures}` (24 byte
  header). Literals are in `__TEXT,__const` / `.rodata`. `iCondBr` with
  `tbz`/`tbnz` tests `inst.bit`.
- Tests: `go test ./internal/arm64` (golden assembly; on an arm64 host
  it builds and runs every golden of internal/llvm against its want.txt
  (35 on 2026-10-08), the bench programs, and `testdata/run/*` against the Go
  backend, all with the leak check; TestGoldenRunShared runs the goldens
  with share probes and assembles their Linux output as ELF). To try a code change by hand, edit
  `<build dir>/arm64/prog-darwin.s` and link it with
  `cc -O2 prog-darwin.s xo_rt.c` (add `-isysroot <Xcode SDK>` when the
  default SDK does not link; on 2026-10-08 the default MacOSX27.0 SDK
  failed with "tapi error: unknown architecture arm64e.x1" and
  `-isysroot /Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk`
  linked). `XO_BUILD_DIR=<dir>` keeps the build directory (and the `.s`)
  where you want it. `regalloc` fails with "reads virtual register n
  before it is written" when MIR uses a register no path defines
  (`gotchas/arm64-mir-param-without-moves.md`).

## Go interop (prototype)

Last verified: 2026-10-08. Decision 0041 is still Proposed; this is the
experiment of its work queue item 6. Results and coverage:
`bench/results/go-interop-2026-10-08.md`.

- Syntax: `use go "<Go import path>" [as name]` (`ast.UseDecl.Go`; the
  lexer reads `go` as an import path and the parser takes the string after
  it). Default local name: the Go package name. Not in `spec/core.md` yet.
- Binding (`internal/gobind`, `xo bind`): runs `go list -e -export -deps
  -json` with `CGO_ENABLED=0 GOWORK=off GOFLAGS=-mod=mod` in the binding
  workspace `<cache>/_mod` (a Go module `xogobind`; third party packages
  are added with `go get <path>@<pin or latest>`), reads export data with
  `go/importer.ForCompiler(fset, "gc", lookup)`, and writes
  `<cache>/<path>.xo` (declarations, signature mode, `use std/go`, every
  function `uses go`; skipped names listed as `// skip: name: reason`
  header lines) and `<cache>/<path>.go` (package `rt` adapters named with
  the prefix `Go_<path with _>_`). Files are written to a temporary name
  and renamed, so parallel builds never read a partial binding.
- Cache: `$XO_GOBIND_DIR`, else `<xo.mod dir or module dir>/.xo/gobind`
  (`.xo/` is in `.gitignore`; Go and the Xo module walk skip dot
  directories). A cached binding is reused unless its header has another
  `xo-bind: v<N>` (gobind.Version) or another module version than the pin.
  Pins: `require go <module> <version>` lines in xo.mod.
- Checker (`internal/check/load.go` `loadGoImport`): binds on demand, loads
  the declaration file as a module `go:<path>` with `IsStd` set and
  `Module.Go` holding the binding. XO1101 when binding fails, XO1102 when
  a name or `Type.method` is in the binding's skip list. Effect label `go`
  (effects.go).
- Codegen: `stdPrefix` returns the binding prefix, so calls lower as std
  calls. `gen.Generate` adds `rt/xo_gobind_<path>.go` per binding and
  writes the workspace's `require` lines into the build `go.mod` and its
  `go.sum`. Runtime helpers: `std/rt/gointerop.go` (`Go_Error`,
  `GoBoundary` panic to fault, `GoContext`, `GoListOf`/`GoSliceOf`/
  `GoMapOf`/`GoMapToGo` copies, `GoCollect` for iterators).
- Tests: `internal/gobind` (names, pins, mapping, cache staleness),
  `internal/check/rules_go_test.go`, `internal/gen/gobind_test.go`
  (`TestGoBindingSignatures`: 20 std packages, every declaration's Go
  signature as gen calls it equals the adapter's), goldens `go_*` and
  `test_go_interop`, `cmd/xo/bind_test.go`, `std/rt/gointerop_test.go`.
  The golden `go_uuid` needs `github.com/google/uuid@v1.6.0` from the
  network or module cache and is skipped when `go mod download` fails. The
  golden and checker tests set `XO_GOBIND_DIR` to a temporary directory.

## Runtime (std/rt)

Last verified: 2026-10-08.

- `core.go` Option/Result/frames/tuples/Box; `arith.go` checked integer
  math; `fault.go` faults and contracts; `list.go`, `map.go` value
  semantics collections: List (a header over a backing array whose block
  has a claimed length and a sticky shared flag: append in place when the
  list ends at the claimed length; element writes, `pop`, `insert`,
  `remove_at` in place while the block is not shared, else copy first and
  mark the copied elements; own marks from generated code, soundness
  argument in the comment on `List`; since 2026-10-08) and Map/Set (append only entry log
  plus an atomic open addressing index; a write appends in place when the
  value's log length equals the claimed length, otherwise it forks a child
  table in O(1); soundness argument in the comment on `Map`, see
  `gotchas/map-copy-claims-log.md`; since 2026-10-08); `show.go` Eq/Hash/Cmp/Debug/Display; `str.go`, `time.go`,
  `option.go` prelude methods; `mono.go` typed helpers of the default
  lowering (`AddI`, `SubI`, `MulI`, `DivI`, `RemI` on int64 with panic
  valued faults, the constant operand forms `AddIUp`, `AddIDown`,
  `SubIUp`, `SubIDown`, `MulIK`, `MulKI`, `CmpO`, `List.AtM`, `List.AtU`;
  same results and fault messages as the generic forms, checked by
  `mono_test.go`); `core.go` `Iface` and `RegAdapter` (interface adapters
  registered by package main for generic code).
- `sched.go` (2026-10-08): threads, the deterministic scheduler, the
  deadlock monitor and XO-F001 reports, `xo dump` (see "Deterministic
  scheduling and live inspection").
- `task.go`, `scope.go`, `chan.go`, `sync.go`: the cancellation tree,
  scopes, tasks, channels and `select` with explicit wait queues, Mutex,
  Atomic, Semaphore, `task.*` (`once.go`: `task.Once`). `Mutex.with` records its acquisition site
  as program counters, resolved to `file.xo:line` only for XO-F004
  (2026-10-08).
- Channel loops, `task.Once`, `task.map_concurrent_until` (2026-10-08,
  decision 0043, core 3.5 and 7.3, std.md 7). Checker: `iterElem`
  (internal/check/stmt.go) gives `Chan[T]` the element type T; patterns
  are checked as in other `for` loops (irrefutable). Codegen
  (`forStmt`, internal/gen/fn.go): `for { rv := ch.M_recv(tk); if
  !rv.Some { break }; ... }`, so every receive is a cancellation point
  that unwinds. `once.go`: `Task_Once[T, E]` is a handle (a pointer to
  `onceState`; `XoEq` compares identity); `get[fx]` has no own non effect
  type parameter, so it is the Go method `M_get`
  (`gotchas/std-method-effect-only-go-method.md`). The first caller runs
  f in its own task, the others queue as waiters; the outcome (value,
  error, or fault) is stored and every waiter shares it. A runner that
  unwinds (cancellation, `proc.exit` in tests) hands the run to the first
  waiter still waiting, or clears the running flag when there is none. A
  fault in f is shared: later callers fault with the same message (the
  spec does not say; this is the choice of 2026-10-08).
  `Task_map_concurrent_until` (sync.go) runs the shared `mapConcurrent`
  loop in a child scope, waits with `awaitDeadline` (shared with
  `with_timeout`), and keeps a result only if it was stored before the
  timer callback set `expired` (one mutex, so the cut is the instant the
  timer fires on the real or virtual clock); the rest are canceled and
  give `Err(TimedOut)`. A fault in an item is raised in the caller.
  `mapConcurrent` rechecks cancellation after acquiring a permit
  (`gotchas/permit-handoff-after-cancel.md`; this also stops
  `map_concurrent` from starting items once canceled). Tests: checker
  `rules07_test.go`; goldens `chan_loops`, `test_once`,
  `test_map_until` (virtual clock), all three in `TestGoldenRace`.
- `arena.go` (2026-10-08): request scoped arenas for `--arenas`. The HTTP
  server takes an `Arena` from a bounded channel free list (128 kept; not
  a sync.Pool, `gotchas/sync-pool-drops-on-gc.md`) per request, puts it on
  the request `Task` (`Task.arena`; scope bodies share it, spawned tasks
  never get it), and resets it after the response is written. It holds
  the request's header, parameter, and query Map tables, the body bytes,
  an arena response's JSON bytes and header table, and typed bump slabs
  for collection temporaries; buffers over 64 KB are not kept. Routes not
  marked confined, and every route behind middleware, get `heapCopy` of
  the request. Soundness argument: the comment on `Arena`. `world.go`: the virtual clock of tests (it
  advances when every task is blocked; wakers count the woken task as
  running first). `main.go`: `RunMain` (root scope, SIGINT/SIGTERM, exit
  status 0/1/70, crash bundles).
- `stop.go` (2026-10-08, decision 0042, core 7.5): two phase shutdown.
  `stopState` belongs to one program run (`RunMain` or
  `testing.run_main`) and every task of the run points to it
  (`Task.stop`, inherited by scope bodies, spawned tasks, and HTTP
  request tasks). Signals are counted: the first sets `requested`, closes
  the `task.stopping()` channel and an `event` for runtime waiters, and
  starts the grace timer (default `rt.DefaultGrace` 30s,
  `Proc.set_grace`; set after the stop it moves the deadline to stop
  time + d; `0s` cancels at once); the grace timer or the second signal
  cancels main's root scope; the third calls `os.Exit(130)` (real) or
  makes `run_main` return 130 and record it as `exit_code()`. Outside a
  run `task.stop_requested()` is false and `task.stopping()` never
  closes. `serve` (real): on the stop, `http.Server.Shutdown` with a
  context canceled when the serving task is canceled, then `Close` and
  cancellation of the request tasks (they hang below a per server node);
  in tests the in memory listener stops routing (new requests fail with
  `ClientErr.Connect`) and waits for requests in
  flight, which run with the server's stop state. Tests:
  `FakeProc.signal_after(d)` queues one signal per call (d from the start
  of the next `run_main`, or from now when the program calls it);
  `set_grace` before `run_main` sets the next run's grace. Timers of the
  virtual clock run the stop logic on a goroutine counted as a running
  task (`stopState.after`, `gotchas/virtual-timer-callback-holds-world-lock.md`).
  Golden `test_shutdown` (also in `TestGoldenRace`, which builds `test_*`
  directories in test mode), real process tests `TestSignal*` in
  `internal/gen/build_test.go` on `testdata/signal` (SIGTERM to the
  test's own child by PID).
- `caps.go` real capabilities (fs rooting per core 6.3), `http.go`,
  `json.go` (`JsonW.str` escapes directly since 2026-10-08, matching
  encoding/json with HTML escaping off, `TestAppendJSONString`),
  `log.go`, `testing.go` (test runner, fakes, `run_main`),
  `std_gen.go` (generated), `gointerop.go` (Go interop boundary
  helpers and `go.Error`, 2026-10-08).

## Agent interfaces (2026-10-08)

Last verified: 2026-10-08.

`xo mcp` (internal/mcp) is a Model Context Protocol server: JSON-RPC 2.0 on
stdin and stdout, one message per line, protocol revision 2025-06-18. It
answers `initialize`, `ping`, `tools/list`, and `tools/call`, and ignores
notifications. Tools, each with a JSON input schema and a compact JSON text
result (`isError` true for usage errors):

| Tool | Arguments | Result |
|---|---|---|
| `check` | `dirs` | `{ok, errors, warnings, modules, diagnostics}` (diagnostics with notes and fixes, core 10.2) |
| `query` | `kind` (type, def, callers, effects, holes, api), `arg`, `dir` | the `xo query --json` answer |
| `edit` | `op` (rename, add-field, change-sig), `args`, `dir`, `dry_run` | the `xo edit` summary `{op, target, ok, written, reason, files, edits, diagnostics, sites}` |
| `explain` | `code` | `{code, severity, title, summary, detail, example, fix}` |
| `fmt` | `paths`, `write` (default false: report only) | `{changed, unchanged, written, diagnostics}` |
| `test` | `dirs`, `run`, `seed`, `holes` | `{ok, passed, failed, skipped, failures}`; `errors` (human text) when the tests do not build |
| `build` | `dirs`, `targets`, `out` | `{ok, binaries, diagnostics}` |

Each tool runs the CLI command line in process with `--json` and reshapes the
output, so it answers what the CLI answers. Client config example:
`{"command": "xo", "args": ["mcp"]}`.

`xo serve` (internal/serve) listens on a unix socket
(`$XO_SERVE_SOCKET`, default `<user cache dir>/xo/serve.sock`, so
`~/Library/Caches/xo/serve.sock` on macOS) and answers `xo check` and
`xo query`. The CLI forwards those two commands when the socket answers
(dial timeout 300 ms) and checks in process otherwise; `XO_NO_SERVE=1`
turns forwarding off. `xo serve stop` shuts the server down through its own
request; `xo serve status` prints cache hits, misses, and entries.

Cache: whole check results (query.Session), keyed by the client's working
directory and the directories checked, at most 16 entries (least recently
used dropped). An entry is reused when every .xo and xo.mod file under the
directories, plus every file of the user modules it loaded, has the same
modification time and size, and no source file appeared. Any change rechecks
the whole program; there is no per module incremental recheck yet. Requests
run one at a time (the server changes to the client's directory for each).
A cached check of a small module took 0.26 ms against 3.3 ms uncached in
`TestServe` (2026-10-08).

Protocol: one JSON line each way, `{"op": "run"|"ping"|"stats"|"shutdown",
"args": [...], "cwd": "..."}` answered by `{"code", "stdout", "stderr",
"cached", "error", "stats"}`.

## Known gaps (2026-10-08)

- LLVM backend prototype (2026-10-08): the subset above only (no Map,
  Set, Bytes, Rune, Duration, sized integers, interfaces, Display of
  structs, `defer`, contracts, tasks, std modules, `use go`, `xo test`,
  `var` parameters of function values, methods as values). Memory is
  reference counted; counts are not atomic. `==` on deeply nested
  recursive values recurses on the C stack. No error traces, crash
  bundles, or debug info; host only. What a full backend
  needs is listed in `bench/results/llvm-prototype-2026-10-08.md`.

- Deterministic scheduling (2026-10-08): a loop with no scheduling point
  keeps the turn forever (`gotchas/busy-loop-starves-deterministic-scheduler.md`);
  there is no preemption at loop back edges. `xo run --seed` reproduces
  interleavings only while real timers and I/O do not race with the
  program. `--explore` tries consecutive seeds with a uniform choice at
  each point, no smarter search (PCT, partial order reduction). Only
  plain tests are explored, not property tests or contract fuzzing.
  Deadlocks that involve a task blocked in I/O or a busy task are not
  reported, and a cycle through a Semaphore has no holder edge. The dump
  is not available on Windows (no SIGQUIT).

- Two phase shutdown in tests (2026-10-08): `run_main` runs main on the
  test's own goroutine, so a third signal cannot end it immediately as
  `os.Exit(130)` does; `run_main` returns 130 once main has unwound (a
  shielded `defer` that blocks keeps it waiting, on the virtual clock).
  Requests to an in memory server run on the client's task, so a stop
  (as opposed to cancellation) of the serving program does not cancel
  them; they only see its `task.stop_requested()`.

- HTTP request cost (2026-10-08, `bench/results/perf-arenas-2026-10-08.md`):
  task 01 under load allocates 13.1 KB and 97 objects per request (6.1 KB
  and 68 with `--arenas`) against Go's 3.0 KB and 31; throughput is 0.43x
  of Go (0.6x with arenas). Remaining sources: `JsonEnc` boxing fields
  through `any`, encoding/json token parsing of bodies, `Mutex.with`
  copying the value to the heap, the router splitting the path per route
  tried, re-parsing the request URL, and the per request Task.

- Generic code (2026-10-08): with the default lowering generic workloads
  run 0.49x to 1.08x of Go generics (`--no-mono`: 1.6x to 16.9x, `==`/`<`
  on a type parameter box both operands and constraint method calls
  convert to an interface). Polymorphic recursion is rejected by Go in
  both modes. An interface adapter for a concrete type that exists only
  inside generic code is not registered for the `--no-mono` lowering
  (`gotchas/generic-constraint-no-adapter.md`).
- Checked arithmetic (2026-10-08): the generic `rt.AddS` family
  (`--no-mono`, sized integer types such as Int32) still faults through an
  out of line call and does not inline; the range analysis has no
  relational facts beyond list lengths and does not follow values across
  calls (a function's result is unbounded at its callers).

- Regression tests (core 9.2): `xo test` passes `XO_REGRESSION_OUT` to the
  test binary; `std/rt/regress.go` records shrunk contract fuzz failures and
  `cmd/xo/regress.go` writes them to the module's `regressions_test.xo` after
  type checking a copy. Property test failures are not written.
- `Map.put` in update heavy loops is no longer a gap (2026-10-08): 3M
  puts over 50k keys plus 3M gets take 142 ms against Go's 45 ms (3.2x,
  was 48x); `Map.update` loops 4.4x. Numbers:
  `bench/results/perf-map-2026-10-08.md`, `perf-mono-2026-10-08.md`.
- List writes (2026-10-08, `bench/results/perf-list-2026-10-08.md`):
  element writes, `push`, `pop`, field updates of elements, `insert`,
  and `remove_at` are in place for the unique owner (sieve 1.4x of Go,
  nbody 1.6x, sort 1.0x, List workloads 1.0x to 1.5x). Remaining costs:
  the shared flag is sticky, so the first write after any copy (including
  `return xs` of a local) copies the list even if the other copy is dead;
  writes through a Map value, a box, `Map.update`, or `Mutex.with` copy
  the list every time (push through them stays O(1)); `--no-mono` marks
  type parameter values through `rt.Own`, which allocates; lists grow by
  doubling (sieve peak RSS 68 MB against Go's 48 MB).

## Go build cache (2026-10-08)

Xo builds and bindings run Go with `GOCACHE` set to `$XO_GOCACHE` (default
`<user cache>/xo/gocache`), trimmed by least recent use to stay under
`XO_GOCACHE_MAX` bytes (default 8 GiB). Entries younger than 2 hours are never trimmed, so concurrent builds keep their files. See `gotchas/go-build-cache-growth.md` and `gotchas/go-cache-trim-races-builds.md`.

