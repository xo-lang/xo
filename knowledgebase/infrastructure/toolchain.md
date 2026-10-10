# Toolchain and layout

Last verified: 2026-10-09

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

## Versions and releases

`xo version` prints the toolchain version (`cmd/xo/version.go`), the
git revision, and the host. The toolchain minor equals the spec draft;
releases are git tags `v0.N.0` with a `CHANGELOG.md` entry, and only the
owner declares 1.0 (decision 0089). First tag: v0.7.0. Last verified:
2026-10-09.

## Commands

Run from the repo root.

```
go build ./...                           # build the compiler packages and std
go test ./...                            # all tests (lexer, parser, checker, spec examples, corpus, CLI, golden programs, race, cross compile)
go vet ./...
gofmt -l internal cmd std                # must print nothing
.tools/bin/staticcheck ./...             # must print nothing
go run ./cmd/xo parse [--json] [--ast] [--sig] <files or dirs...>
go run ./cmd/xo check [--json] [--no-gc] [--fix] <dirs...>   # --fix: apply machine fixes first (decision 0096)
go run ./cmd/xo build [--targets=all|os/arch,...] [-o out] [--json] [--no-mono] [--no-arenas] [--backend=go|llvm|arm64] [--no-gc] [--hybrid] [--ci] [--release [--error-args=false]] [dir...]   # --ci: fail before building on an unformatted file or a dbg left in code (core 10.5, 11.4; 2026-10-09); --release: requires(debug)/ensures(debug) compiled out, XO1308 on workspace overrides (core 8, decision 0099; --ci does not imply it); --error-args=false: error frames without parameter values (decision 0092)
go run ./cmd/xo run [--seed=N] [--record=trace.xotrace] [--holes] [--no-mono] [--no-arenas] [--backend=go|llvm|arm64] [--no-gc] [dir] [-- args...]
go run ./cmd/xo build --no-gc .          # no garbage collector: arm64 on arm64 hosts, llvm otherwise (--backend picks); XO1201 for what the native backends cannot build, before code generation (decision 0083; also check, run, test)
go test ./internal/build -run TestNoGCCheckAgrees    # the no GC check agrees with LLVM and arm64 code generation on every golden, bench, selfhost, and example program (seconds; also -short)
go run ./cmd/xo test --no-gc [-v] [--backend=llvm|arm64] examples/notes-service   # a native test binary, one process per test (XO_TEST=i, XO_TEST_SEED=s) in the C test world; same report as the Go backend
XO_TEST=3 XO_TEST_SEED=1 <build dir>/arm64/bin/<module>.test   # run one test of a native test binary by hand: exit 0 passes, XO-TEST-FAIL on stdout and exit 1 fails, XO-TEST-LOG lines are its log records
go test ./cmd/xo -run 'TestNoGC(Tests|TestLeaks|NotesServer)'   # native test reports vs the Go backend (cmd/xo/testdata/nogc, examples/notes-service), no live allocation at exit, and notes-service served natively on a socket with SIGTERM shutdown (about 40 s)
go run ./cmd/xo replay [--dir=<module dir>] <trace.xotrace | crash.xocrash>   # rebuild and rerun offline from the trace (core 11.3)
go run ./cmd/xo test [--seed=N] [--explore=N] [--run=text] [--race] [--real] [--cases=N] [--json] [-v] [--holes] [--no-mono] [--no-arenas] [--backend=go|llvm|arm64] [--no-gc] [dir...]
go run ./cmd/xo dump [--timeout=5s] <pid>                        # task tree of a running Xo program (core 11.4)
go run ./cmd/xo bind [--dir=<project dir>] [--json] <go import paths...>   # bind Go packages for `use go` (Go interop)
go run ./cmd/xo fmt [--check] [--json] [files or dirs...]        # canonical layout (core 10.5)
go run ./cmd/xo vet [--json] [dirs...]                           # advisory checks, never errors
go run ./cmd/xo fix [--dry-run] [--json] [--from=draft-0.N] [--edition=YYYY] [dirs...]  # apply check fixes, migrate
go run ./cmd/xo query <kind> [--json|--text] [--dir=<dir>] <arg> # type, def, callers, effects, holes, api, imports, deps (core 10.3)
go run ./cmd/xo edit [--dry-run] [--dir=<dir>] <op> <args...>   # rename, add-field, change-sig (core 10.4); JSON summary, exit 1 when rejected
go test ./internal/edit -run TestGolden -update          # rewrite internal/edit/testdata/*.golden
go run ./cmd/xo doc [--json] [--all] [--dir=<dir>] [module]      # Markdown docs from doc comments
go run ./cmd/xo explain [--json] <code>                          # long form explanation of a diagnostic code
go run ./cmd/xo mcp                                              # MCP server on stdio for agents (see "Agent interfaces")
go run ./cmd/xo lsp                                              # LSP server on stdio for editors (see editors.md)
go run ./cmd/xo serve [--socket=path] [stop|status]              # resident checker; check and query use it when it answers
go test ./internal/format -run TestGolden -update        # rewrite internal/format/testdata/*.golden
go test ./internal/gen -run TestGoldenNoMono -golden-update   # rewrite want.txt of the golden programs
go test ./internal/gen -run TestGoldenRelease [-golden-update]   # testdata/release/*: want.debug.txt, want.release.txt, want.release_noargs.txt (xo build --release [--error-args=false], decisions 0099, 0092); -golden-update rewrites only the files that exist
go test ./internal/llvm -run TestGolden -golden-update   # rewrite want.txt of the LLVM backend golden programs (from the Go backend's output)
go test ./internal/ssa -update                       # rewrite the SSA goldens (testdata/golden/*/want.ssa and want.rc.ssa, testdata/bench/*.ssa and *.rc.ssa)
XO_SSA_DUMP=/abs/dir [XO_SSA_RC=1] go test ./internal/ssa -run TestDump -v   # print a program's SSA (after InsertRC), or every builder and verifier error
XO_LEAKCHECK=1 ./prog                                  # an LLVM backend binary prints `xo leakcheck: N live of M allocations` at exit
go test ./internal/llvm -run 'TestRuntimeThreads|TestGoldenThreads'   # shared counts and the runtime under ThreadSanitizer (decision 0058; about 10 s)
go test ./internal/llvm -run 'TestRuntimeTasks|TestGoldenTasks|TestDeadlock'   # the task runtime and the task goldens under ThreadSanitizer (decision 0068; about 15 s)
go test ./internal/llvm -run 'TestRuntimeEventLoop'   # fibers, timers, sockets in C under ThreadSanitizer, 1 and 3 workers, then -O2 (decision 0069; about 10 s)
go test ./internal/llvm -run 'TestNativeSignal'       # SIGTERM two phase shutdown of testdata/signal built with the LLVM and arm64 backends (core 7.5; about 5 s)
go test ./internal/llvm -run 'TestJSONParseDiff|TestHTTPParseDiff'   # native JSON parser vs std/rt's jsonParse, native HTTP/1.1 reading and writing vs a net/http server (decision 0074; AddressSanitizer and UBSan; a few seconds)
go test ./internal/llvm -run TestTLSDiff           # native HTTPS vs the Go backend: client errors, SNI, ALPN, versions, plain HTTP on the TLS port, SIGTERM (decisions 0105, 0106; about 15 s)
go test ./internal/llvm -run TestCryptoDiff        # native std/crypto (std/llrt/xo_crypto.c) vs Go's crypto, RFC 9106 vectors, x/crypto Argon2id outputs, std/rt's verify_password on mutated hashes (decision 0083; AddressSanitizer and UBSan; about 3 s)
XO_HTTP_DIFF_SEED=7 XO_HTTP_DIFF_N=30000 go test ./internal/llvm -run TestHTTPParseDiff   # more random and mutated connections
go test ./internal/llvm -run '^$' -fuzz FuzzHTTPDiff -fuzztime 60s -parallel 6   # Go's fuzzer on the same comparison (more workers than about 6 starved it on 2026-10-08: "context deadline exceeded")
XO_FUZZ_SECONDS=60 FUZZ_SEED=3 go test ./internal/llvm -run TestHTTPFuzz -v   # std/llrt/testdata/http_fuzz.c: mutated HTTP and JSON under AddressSanitizer and UBSan (5 s by default, 1 s with -short)
go test ./internal/llvm -run 'TestGoldenASan'         # net_echo and the http_* goldens with AddressSanitizer and UBSan, 1 and per CPU workers
go test ./internal/check -run TestNativeModules       # std/native implementation modules check without diagnostics
XO_THREADS=1 ./prog                                   # a native binary with one worker thread (default: one per CPU)
XO_RTSTATS=1 ./prog                                   # native: scheduler counters on stderr at exit (spawns, steals, wake ups, idle sleeps, spin rounds, fiber parks, thread waits, slow locks; decision 0077) and socket counters (net_read, net_write, net_again = EAGAIN results, poll_return, poll_event, poll_wake; 2026-10-09)
sample <pid> 5 -mayDie -file prof.txt                 # native server under load: macOS sampling profiler, no root; read "Sort by top of stack" (idle threads show as __ulock_wait and kevent); the llvm build has Xo function names (x.<fn>), arm64 folds some into xo_entry (last verified 2026-10-09)
clang -O1 -g -fsanitize=thread std/llrt/testdata/rc_threads.c -o /tmp/rct && /tmp/rct   # the C stress test by hand (add -isysroot <SDK> when the default SDK does not link)
go test ./internal/arm64 -run TestGoldenAsm -update     # rewrite the golden assembly internal/arm64/testdata/{fib,sieve}.{darwin,linux}.s
go run ./cmd/xo build --backend=arm64 --targets=linux/arm64 .   # emit <build dir>/arm64/prog-linux.s (linked only on a linux/arm64 host)
XO_RANGE_SEEDS=25 go test ./internal/gen -run TestRangeDifferential   # slow: more random programs, range analysis vs all checked
XO_LIST_SEEDS=40 go test ./internal/gen -run TestListModel   # slow: more random List value semantics programs vs a deep copy model (XO_LIST_DUMP=<dir> keeps them)
XO_COMPILE_PROPS=1000 go test ./internal/gen -run TestCompileProperty   # every checked program builds (decision 0110): random programs over the optional syntax, built on Go and --no-gc and run alike (24 with -short, 150 in the full suite, about 4 min for 1000; XO_COMPILE_PROP_SEED=S one seed, XO_COMPILE_PROP_KEEP=<dir> keeps them; last verified: 2026-10-09)
go test ./internal/gen -run TestStdTypes -update        # regenerate std/rt/std_gen.go from std/*.xo
go run ./cmd/xo lexdump [--bench=N] <files...>              # hidden: canonical token dump of the Go lexer (or lexing time), same text as selfhost/lexer
go run ./cmd/xo run selfhost/lexer -- [--bench=N] <files...>   # the self hosted (Xo) lexer: same dump or timing
go run ./cmd/xo build --backend=arm64 -o .tools/xolex selfhost/lexer   # native build of the Xo lexer (also --backend=llvm)
go run ./cmd/xo test selfhost/lexer                     # the Xo lexer's own tests (20, 6 of them property tests)
go test ./internal/selfhost                             # Xo lexer (built with the go, arm64, and llvm backends) vs Go lexer byte for byte: repo .xo files, spec blocks, runes, 3000 mutated inputs (300 with -short); XO_SELFHOST_SEED, XO_SELFHOST_FUZZ=N, XO_SELFHOST_RUNE_STEP=1 (all runes), XO_SELFHOST_KEEP=<dir> keeps the inputs; one backend: -run 'TestSelfhostDifferential/arm64'
go test -short ./...                                    # about 10s: skips race, cross compile, CLI build, replay, the --no-mono and --no-arenas golden lowerings, and the notes-service arena differential; use per commit, run the full suite before merging (about 2.5 min)
scripts/linux-test.sh [arm64|amd64] [go test args...]  # the suite in a Linux container (Docker; see "Linux (containers)"): arm64 vet plus the full suite natively, amd64 -short plus internal/llvm under emulation
bench/nightly.sh                                        # full suite with XO_RANGE_SEEDS=25 XO_LIST_SEEDS=40, -race on std/rt, vet; log in bench/results/nightly/DATE.log (gitignored), one line per run in summary.txt; schedule with cron (not installed)
go test -race ./std/rt -run 'TestMutex|TestSched'      # runtime lock and scheduler under the race detector (Mutex.with_read readers, writers, cancellation; last verified 2026-10-08)
go test ./std/rt -run '^$' -bench . -benchmem           # runtime micro benchmarks (Map put/get/update/iter/copy/fork, Set, Eq, Hash)
go test ./std/rt -run '^$' -bench 'Order|Frames' -benchmem   # spec audit benchmarks: Map iteration order (decision 0091) and error frame cost (decision 0092: eager before, impl as emitted now, FramesPrint the cost at print time); about 90 s (last verified 2026-10-09)
go test ./internal/diag -run 'TestSpecLists'           # core 10.6 lists exactly the codes of internal/diag/codes.go and the XO-F fault codes the runtimes raise
python3 bench/harness/score.py solutions --claude        # rescore the stored agent benchmark outputs with the current toolchain (bench/harness/README.md)
python3 bench/harness/run_openai_compatible.py --out runs/<model>   # rerun the agent benchmarks with any OpenAI compatible model
bench/perf/run.sh                                       # Xo vs Go collection, List write, and generic workloads (see Benchmark tooling)
MONO=1 bench/perf/run.sh                                # Xo --no-mono and Xo (default, --mono) columns
bench/perf/http/run.sh                                  # task 01 HTTP load: Go vs Xo --no-arenas (config xo) vs Xo with arenas (config xo-arenas, the default build; see Benchmark tooling)
bench/perf/llvm/run.sh                                  # compute and string workloads: Go vs Xo (Go backend) vs Xo --backend=llvm (RSS=1: peak memory table)
bench/perf/llvm/native.sh                               # same workloads: Go vs Xo --backend=llvm vs Xo --backend=arm64, time and peak RSS (arm64 host)
bench/perf/hybrid/run.sh                                # Xo (Go backend) vs xo build --hybrid vs --backend=arm64: median wall ms and cycles of 7 interleaved runs (arm64 host, decision 0084)
bench/perf/conc/run.sh                                  # concurrency: Go vs Xo (Go backend) vs llvm vs arm64 (and arm64 XO_THREADS=1); medians of wall, CPU, cycles, interleaved (see Benchmark tooling)
python3 bench/perf/conc/profile.py p.trace 40 ./bin     # summarize an xctrace CPU Profiler trace by function (PROFILE_FUNC=name: hot offsets in one function)
go test -run '^$' -fuzz=FuzzParse -fuzztime=60s ./internal/parser/   # optional fuzzing
go test ./internal/check -run TestGolden -update          # rewrite golden diagnostics after an intended change
go test ./internal/fix -run TestHabitGolden -update       # rewrite want.json and fixed.xo of the habit cases (internal/fix/testdata/habits, decision 0087); review the diff
python3 -I bench/habits/measure.py .tools/bin/xo internal/fix/testdata/habits golden   # habit cases: errors, fixes, green after xo fix (also snap and drafts modes; last verified 2026-10-09)
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
and message, before and after). (`xo build --ci` exists since 2026-10-09.)

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
`go build -trimpath` with `CGO_ENABLED=0` per target. A program that binds
a Go package with callback adapters (`NewGoCallback` in an
`rt/xo_gobind_*.go`) also gets `-tags=xo_gotimers
-ldflags=-checklinkname=0`, which link `runtime.timeSleepUntil` into std/rt
for deadlock reports through Go calls (decision 0073); `XO_GOTIMERS=off`
drops them, and a failed link of that symbol is retried without them with
one warning line (`xo build: warning: deadlock detection through Go
callbacks is off ...`; gotcha `go-runtime-timer-linkname.md`; Last
verified: 2026-10-08 with Go 1.26.1). Every Go backend build also passes
`-gcflags=all=-dwarf=false` and `-ldflags=-s -w` (merged with the
`-ldflags` above): no DWARF in the cached objects (a third of each) or the
binary, and no symbol table. Xo's stack traces, faults, crash bundles, and
`xo dump` read the pclntab through the Go runtime, which stays; only a
native debugger (Delve) needs DWARF, and `XO_GO_DWARF=1` keeps both. A
program (not a test binary) that does not import std/http, directly or
through std modules, is built with the tag `xo_nohttp` (gen.Program.Tags),
which swaps the real server and client in `std/rt/http_net.go` for the stubs
in `http_nonet.go`, so the binary does not link Go's net/http (hello world
3.6 MB instead of 4.9 MB; bench/results/binary-size-2026-10-09.md; Last
verified: 2026-10-09). It prints one
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
the module paths, mode, lowering, arenas on or off, and layout tag>/` (stable, so Go's build
cache is reused; the layout tag keeps older xo binaries out, see
`gotchas/build-dir-shared-across-xo-versions.md`): `go.mod` (module
`xoprog`), `rt/` (the runtime, copied from the embedded `std/rt`),
`m/<module ident>/xo.go` per Xo module, `main.go`, `xo_adapt.go`
(package main: interface adapters registered for generic code, often
empty), and (build mode, since 2026-10-08) `xo_buildinfo.go` (package
main: `rt.SetBuildInfo` with the build id, source hashes, main module
directory, and options, `build.Info`). Last verified: 2026-10-08. Crash bundles go to
`$XO_CRASH_DIR` or the system temporary directory. `XO_GO` overrides the go
command (default: `go` on PATH). A program that calls `time.zone` also
gets `xo_tzdata.go` (package main: `rt.TZEmbed` per constant zone name,
or `rt.TZFull` over `xo_zoneinfo.zip` embedded with go:embed when a name
is known only at run time, or `rt.TZUseSystem` with `--tzdata=system`).
The zone data is `$(go env GOROOT)/lib/time/zoneinfo.zip` of the go
command the build uses; `XO_ZONEINFO=<file>` names another zip. Last
verified: 2026-10-09.

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
- `bench/size/measure.sh <dirs>` (2026-10-09): binary size, Go build
  cache growth, peak RSS, and wall time of `xo build` per program against
  a fresh cache with the runtime warmed (`MODE=test` for `xo test`,
  `XO=<binary>` to compare builds); results in
  `bench/results/binary-size-2026-10-09.md`. Last verified: 2026-10-09.
- `bench/go/strict-check.sh <dir>`: vet, staticcheck, errcheck, nilaway,
  exhaustive, `go test -race`. Analyzers come from `$XO_TOOLS_BIN` when set,
  else the checkout's `.tools/bin` (a worktree has none; point
  `XO_TOOLS_BIN` at the main checkout's). Last verified: 2026-10-08.
- `bench/harness/` (2026-10-08): reruns the agent benchmarks with any model
  and scores them; Python 3 standard library only, no third party packages.
  `prompts.py render|all|tokens` renders the per arm and planted author
  prompts (the Xo ones include `spec/core.md` and `spec/std.md`) and
  estimates tokens; `run_openai_compatible.py --out DIR` drives any OpenAI
  compatible endpoint (`OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL`,
  optional `OPENAI_EXTRA_BODY`) through the feedback loop, up to
  `--max-runs` runs; `score.py solutions DIR|--claude` and
  `score.py planted DIR|--claude` rerun every cell in a temporary copy and
  print the three arm and planted tables. When a cell's first run sources
  exist (`<cell>/.attempts/run-1`, or `<DIR>/history/<task>/1` for a run
  made another way), `solutions` also reruns them as is and, for Xo, after
  `xo fix` ("First try now", "First try after `xo fix`", and a "First
  runs" table; since 2026-10-09). The scorer builds `xo` from the
  checkout into `$TMPDIR/xo-harness/xo` unless `XO_BIN` is set, and finds
  the analyzers through `XO_TOOLS_BIN`, `.tools/bin`, or the main
  checkout's `.tools/bin`. Self test: `python3 bench/harness/test_harness.py`
  (also `go test ./internal/benchharness`, about 7 s before 2026-10-09 and about 35 s since, with the Xo `xo fix` case; skipped without
  python3; uses a fake endpoint on 127.0.0.1). Scoring all stored Claude
  solutions took about 12 minutes, all planted sets about 60 (measured
  2026-10-08).
- `bench/perf/conc/run.sh` (2026-10-09, decision 0077): five concurrency
  programs (`xo/spawn`: 200 rounds of 1000 discarded tasks; `fanout`: 100
  rounds of 1000 awaited tasks; `pingpong`: 100,000 round trips over two
  unbuffered channels; `mutex`: 8 tasks x 25,000 `with`; `pmap`: 20 rounds
  of 64 Collatz chunks) and idiomatic Go (`go/`, `concperf <name>`).
  Configurations `go`, `xo-go`, `xo-llvm`, `xo-arm64`, `xo-arm64-1`
  (`XO_THREADS=1`); `REPS` (7) rounds interleaved; medians of wall
  (perf_counter), CPU (child rusage, user + sys), and cycles (`/usr/bin/time
  -l`, Darwin). `WORKLOADS`, `CONFIGS`, `KEEP=dir` (keep binaries), `XO`.
  Prints `uptime` first: other processes on the host skew wall time most.
  Profiling: `xcrun xctrace record --template 'CPU Profiler' --output
  p.trace --launch -- ./bin`, then `xcrun xctrace symbolicate --input
  p.trace --dsym ./bin --output s.trace` and `bench/perf/conc/profile.py
  s.trace 40 ./bin` (names the runtime's static functions from `nm`;
  leaf addresses in the export are the PC plus one). ThreadSanitizer runs
  of these programs need reduced sizes
  (`gotchas/tsan-fiber-trace-crash.md`).
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
  CPU percent, peak RSS, GCs per 10k requests, and (since 2026-10-09) the
  server's CPU time per request in the measured window (`ps -o time` of
  its PID before and after, user plus system, microseconds; loadgen's
  `cpu_us_per_req`). `CONFIGS` (default `go xo
  xo-arenas`; since decision 0066 `xo` builds with `--no-arenas` and
  `xo-arenas` with the default; `xo-base` builds with the binary in
  `XO_BASE` in its own build directory and that binary's default,
  `xo-base-arenas` the same with `--arenas`; `xo-arm64` and `xo-llvm` build
  with `--backend arm64` and `--backend llvm`, the native runtime of
  decision 0074; first measured 2026-10-09, results "Native backends"), `XO`, `PERF_OUT`, `PRELOAD`, `PORT` (18080). The load
  is closed loop (`gotchas/closed-loop-latency-confound.md`). Results:
  `bench/results/perf-arenas-2026-10-08.md`,
  `bench/results/perf-throughput-2026-10-08.md`.
- Heap goal of built Xo programs (decision 0065, 2026-10-08): `RunMain`
  keeps the GC goal at or above a floor (default 2 MiB per P, at least
  16 MiB) with GOGC=100 growth above it, by setting the GC percent after
  every cycle (std/rt/gc.go). `gc min_heap|percent|memory_limit` lines in
  the main module's xo.mod change it; `GOGC` (turns the floor off),
  `GOMEMLIMIT`, and `XO_GC_MIN_HEAP` (0: no floor) in the environment win.
  So run.sh at default settings compares Go at GOGC=100 with Xo at its
  floor; pass `GOGC=` to compare equal settings (both get it). allocs.sh
  GC counts are taken at the default floor too. Last verified: 2026-10-08.
- Execution trace (2026-10-08): `XO_TRACE=<file>` writes a `runtime/trace`
  of the whole run when `RunMain` returns (stop a server with `kill <its
  PID>`); read with `go tool trace <file>`. Last verified: 2026-10-08.
- Profiling a built Xo program (2026-10-08): `XO_CPUPROFILE=<file>` writes
  a CPU profile of the whole run and `XO_BLOCKPROFILE=<file>` a block
  profile (where goroutines waited: locks, channels, select) when
  `RunMain` returns, so stop a server with `kill <its PID>` (SIGTERM, a
  clean shutdown), not SIGKILL. Read with `go tool pprof -top -cum
  <binary> <file>`. Last verified: 2026-10-08.
- Allocation profiling (2026-10-08): `XO_MEMPROFILE=<file>` writes the
  `allocs` profile when `RunMain` returns (`XO_MEMPROFILERATE=N` samples
  one allocation per N bytes, 1 for every allocation; read with `go tool
  pprof -sample_index=alloc_objects -lines -top <binary> <file>`, or
  `alloc_space`), and `XO_MEMSTATS=<file>` writes `mallocs`,
  `total_alloc`, and `num_gc` of the whole run. The Go server of
  bench/perf/http writes the same three lines with `MEMSTATS=<file>` on
  SIGTERM. `bench/perf/http/allocs.sh` uses them: per config (`go`, `xo`
  = `--no-arenas`, `xo-arenas` = default, `xo-base` and
  `xo-base-noarenas` with `$XO_BASE`) and route (`get list patch
  createdel cycle`), two fresh servers get N1 and N2 units of the route
  from `loadgen -route <r> -n <n>` on one connection (port 18190), and
  the difference gives allocations, bytes, and GCs per request. The
  counts are deterministic to about 0.1 allocation and do not depend on
  host load. An `xo-base` binary needs the XO_MEMSTATS hook (commit
  8b65b93); for an older checkout apply that commit's std/rt/main.go
  change first. Allocation budgets of the hot runtime paths are pinned
  in `std/rt/alloc_test.go` (skipped where a sync.Pool is involved under
  `-race`, which drops pooled items at random). Last verified:
  2026-10-08.
- Lock hold and wait times (2026-10-08): `XO_LOCKSTATS=<file>` makes every
  `Mutex.with` and `Mutex.with_read` of a built Xo program record how
  long it waited to acquire and how long it held the mutex, per call
  site, and `RunMain` writes a Markdown table (count, p50, p99, mean in
  us) to the file on return (stop a server with `kill <its PID>`;
  `std/rt/lockstats.go`). Off by default (one bool load per
  acquisition). The Go server of bench/perf/http records the same table
  per store operation with `HOLDSTATS=<file>` (written on SIGTERM;
  `bench/perf/http/go/lockstats.go`), so both columns compare directly.
  The samples stay in memory (tens of MB), which changes GC pacing: do
  not read throughput from an instrumented run
  (`gotchas/sample-recording-changes-gc-pacing.md`).
  Last verified: 2026-10-08.
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
- Code placement sweeps for the arm64 backend (decision 0075; Last
  verified: 2026-10-08): `XO_ARM64_LAYOUT=pad=K xo build --backend=arm64`
  puts K nops (never run) before every function label, moving all code;
  build K = 0 to 15 and compare the median cycles over the 16 binaries
  (and 16 copies of one binary as the noise floor) to judge a code shape
  change. Also `func=N` and `loop=N` (log2 alignment of functions and
  innermost loop tops), `fall=1`, `off=K`, `outer=1`. Unset is the
  default placement; a bad value fails the build. The build directory is
  keyed by the program, not the layout: copy `prog-darwin.s` before the
  next build.
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
- `bench/harness/score.py planted DIR|--claude`: the same detection for the
  Go set and both Xo sets in one run, with the current toolchain, for
  planted sets written by any model (see `bench/harness/README.md`). Since
  2026-10-09 a test failure counts as a catch only for a test that passes
  on the unplanted base (`new_test_failures`, also in
  `bench/planted-xo/run_detection_xo.py`; tested by
  `TestPlantedTestCatches` in `bench/harness/test_harness.py`). Last
  verified: 2026-10-09.

## Package layout (P1, implemented parts)

```
cmd/xo/                 CLI entry point; `xo parse`, `xo check`, `xo build`, `xo run`, `xo test`, `xo bind`,
                        `xo fmt`, `xo vet`, `xo query`, `xo doc`, `xo explain` (tools.go), `xo dump` (dump.go),
                        `xo mcp`, `xo lsp`, `xo serve` (agent.go), hidden `xo lexdump` (lexdump.go)
                        `xo fmt`, `xo vet`, `xo query`, `xo doc`, `xo explain` (tools.go), `xo dump` (dump.go), `xo fix` (fix.go)
editors/vscode/          VS Code extension (TextMate grammar, Markdown injection, xo lsp client); see editors.md
internal/token/         token kinds, keywords, Pos and Span (file, line, col, offset)
internal/lexer/         tokens, newline termination (core 1.3), string interpolation parts,
                        doc comments; dump.go: the canonical token dump shared with selfhost/lexer
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
internal/lsp/           `xo lsp`: LSP 3.17 over stdio; rechecks open documents as a check overlay (editors.md)
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
internal/selfhost/      Go side of the self hosted components (tests only): build selfhost/lexer and compare
                        its dump with lexer.Dump (decision 0076)
selfhost/lexer/         the lexer in Xo (decision 0076): token.xo, lexer.xo, dump.xo, main.xo (CLI),
                        lexer_test.xo; Unicode and quoting come from the prelude (decision 0079)
scripts/linux-test.sh   the test suite in a Linux container (arm64 or amd64), image from bench/linux/Dockerfile
                        (see "Linux (containers)")
internal/benchharness/  test only: runs the bench/harness Python self test under go test
internal/gobind/        `use go` bindings: go list + go/importer + go/types -> Xo declaration
                        file and Go adapter (decision 0041 prototype, see "Go interop")
internal/cbind/         `use c` bindings: clang's JSON AST dump of a header -> Xo declaration
                        file and C adapter (decision 0116, see "C interop")
std/                    std declaration files (prelude.xo, go.xo, http.xo, json.xo, log.xo,
                        path.xo, sql.xo, task.xo, testing.xo, time.xo) embedded by std/embed.go
std/rt/                 package rt, the runtime linked into every program (Go, embedded
                        by std/embed.go as std.Runtime and copied into each build)
std/llrt/               xo_rt.c, the C runtime of the LLVM and arm64 backends (embedded as std.LLRuntime),
                        and the files it includes: xo_std.c, xo_unicode.c, xo_bytes.c, xo_json.c,
                        xo_http.c (decisions 0074, 0081), xo_tls.c (TLS on Mbed TLS, decision 0105);
                        testdata/*.c, C tests and differential drivers (not embedded)
third_party/mbedtls/    Mbed TLS 4.1.1 for xo_tls.c (decision 0105, see "TLS library"): src/ (vendored,
                        unmodified), mbedtls.tgz (its compressed copy, embedded), xo/ (configuration)
scripts/vendor-mbedtls.sh  downloads, checks, and vendors an Mbed TLS release
std/native/             std implemented in Xo for the native backends (decision 0074): <name>.xo
                        implements std/<name>; rt.xo, rtjson.xo, rthttp.xo declare C intrinsics
                        (embedded as std.NativeFiles, loaded only for native builds)
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

Build modes (decision 0099, 2026-10-09): `build.Options.Release` (only
in `gen.Build` mode, `Options.release()`) sets `gen.Options.Release`,
`ssa.Options.Release` (LLVM, arm64, `--hybrid`), the build info field
`release`, the build id, and a `release` variant of the build directory
key. `gen.contractOn` and the SSA builder's `contractOn` drop clauses
with `Debug` set. Tests and `xo run` never pass it.
`build.Options.NoErrorArgs` (`--error-args=false`, release only) makes
`internal/gen` emit `rt.Frame0` for every `?` (build info
`no_error_args`, build directory variant `noargs`). Last verified:
2026-10-09.

With `--backend=llvm` (2026-10-08): checker -> SSA with reference
counting (internal/ssa) -> LLVM IR (internal/llvm) ->
`<build dir>/llvm/prog.ll` plus `xo_rt.c` -> `clang -O2 -flto` -> one host
binary. The build directory keeps `prog.ll` for inspection.

With `--backend=arm64` (2026-10-08): checker -> SSA (internal/ssa,
`InsertRC`, verified) -> assembly (internal/arm64) ->
`<build dir>/arm64/prog-<os>.s` plus `xo_rt.c` -> `cc -O2` -> one binary
for darwin/arm64 or linux/arm64 when that is the host; another arm64
target is written as assembly only (`build.Result.Asm`).

Both native links (and every C compile through `build.CompileC`, the
sanitizer tests included) add `-fexceptions -funwind-tables` (2026-10-09,
decision 0085): faults unwind through the runtime's frames and generated
code's (call frame information, call site tables, `xo_personality`), so
no C function may be assumed not to unwind. When linking a kept
`prog-darwin.s` by hand, add the same flags. Last verified: 2026-10-09.

## Code generation

Last verified: 2026-10-08. Rules and choices; details in
`bench/results/gen-run-v0.4.md`.

- One Go package per Xo module (`xoprog/m/x_<module path>`), one file
  `xo.go`; the main module's package also has `xo_main.go` with `XoMain`
  (2026-10-09), the run of `main` that package main calls, so package
  main names no runtime type (see the bullet on runtime generic types). Std modules map onto package `rt`. Go names: types keep their
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
- Runtime generic types carry only protocol methods (2026-10-09):
  `XoEq`, `XoHash`, `XoCmp`, `XoDebug`, `XoJsonEnc`, `XoJsonDec`,
  `XoJsonRead`, `XoGenAny`, `XoMark`, `M_display`, `M_debug`. Prelude
  methods of List, Map, Set, Option, Result, Chan, Secret, and json.Patch
  and the generator's helpers are package level functions:
  `rt.List_len[T](tk, xs)`, `rt.Map_put(tk, &m, k, v)`,
  `rt.List_At(xs, i, loc)`, `rt.List_PushFast(xs, x)` (`rtFuncMethods`,
  internal/gen/decl.go). Go compiles every method of every generic type a
  package mentions, in that package, so methods there cost every
  generated package (`gotchas/go-generic-method-sets-per-package.md`,
  enforced by `TestGenericMethodSets`). These types satisfy Xo interfaces
  through generated adapters (also in the `--no-mono` registry).
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
  plain Go, and `xs[i]` proven in range is `rt.List_AtU`. Other operations
  with a constant operand use one comparison forms (`rt.AddIUp`,
  `rt.SubIDown`, `rt.MulIK`/`MulKI`, plain `/` and `%` by a constant other
  than 0 and -1); the rest call `rt.AddI..RemI` and `rt.List_AtM` (typed,
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
  on the path is made the unique owner of its array (`if
  !rt.List_Mine(p) { p = rt.List_Unshared(p) }`), then
  `rt.List_PtrM(p, i)` or `rt.List_PutM(p, i, v)`; a compound
  assignment uses one address for the read and the write. Through a Map
  value or a box it reads, marks, modifies, and writes back
  (`m[k].count += 1`). A std `var` method on an element (`xs[i].push(x)`)
  gets the element's address; a user `var` method gets a marked copy
  that is written back. The value is computed first when it could write
  the place's root (2026-10-08).
- List ownership (2026-10-08, internal/gen/own.go, rationale and numbers
  in `bench/results/perf-list-2026-10-08.md`): a read of a binding,
  field, or element whose type holds lists is wrapped in an own mark
  (`rt.List_Own(x)`, or a function literal calling generated `XoMark` methods
  for structs, tuples, Options, Results; `rt.Own` for type parameters)
  unless the reading expression borrows it: field and element bases,
  receivers of read only List methods, comparison, `+`, and
  interpolation operands, loop subjects the body does not write, by value
  arguments of user calls without `var` parameters (each checked with
  `mutates`; `var` parameters may alias). Std call results holding lists
  are marked except fresh producers (`freshStd`); closures mark what they
  capture; interface conversions mark. Structs and newtypes whose fields
  hold lists get `func (v *T) XoMark()`. `push` on a binding or field
  path is `if rt.List_CanPush(p) { p = rt.List_PushFast(p, x) } else { p
  = rt.List_PushSlow(p, x) }`. A field read of an element is
  `rt.List_RefM(xs, i, at).F_f` (no copy of the
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
- Request scoped arenas (2026-10-08, internal/gen/arena.go; on by
  default since decision 0066, `--no-arenas` on `xo build`, `run`, `test`
  turns them off, `--arenas` is accepted for older command lines;
  `gen.Options.Arenas`, `build.Options.NoArenas`, build info
  `no_arenas`; off generates exactly the code of before arenas; the
  llvm and arm64 backends ignore them): the generated main calls
  `rt.EnableArenas()`. A
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
  the flag. Tests: `TestArenaEscapeAnalysis`, `TestArenaConfineRandom`
  (160 random handler bodies, confined exactly when no escaping use),
  `TestGolden` (arenas) with `TestGoldenNoArenas` (every golden with
  `--no-arenas`, same want.txt), `TestGoldenRace` with
  `TestGoldenRaceNoArenas`, goldens `arena_escape` and `arena_holes`,
  `TestExampleArenaDiff` and `TestExampleArenaServeDiff`
  (examples/notes-service tests and real server both ways),
  `TestReplayServer` (recorded arena bodies), `TestArenaHTTP2` (std/rt,
  not with -short: the server over a TLS listener with net/http's
  built in h2 client from `httptest`; `rt.arenaPoison` fills released
  arena buffers with `#` so a late reader shows at once) and
  `TestServeNoH2C` (plain listeners speak HTTP/1 only). On HTTP/2 serve
  writes a heap copy of an arena body. Last verified: 2026-10-08.
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
- std/time (2026-10-09, decision 0094, std.md 13): `std/time.xo`;
  `std/rt/timezone.go` (Zone handle, zone registry, a stored zip reader
  so rt does not link archive/zip, `local`), `timecivil.go` (dates by
  day number, `instant` with gap and overlap detection over the offsets
  two days either side), `timefmt.go` (layout parser shared in spirit
  with `internal/check/timelayout.go`, kept equal by
  `TestTimeLayoutAgrees`; format, strict parse). Per use zone data:
  `internal/gen/timezone.go` records `time.zone` calls (constant names
  through `check.ConstString`), `internal/build/tzdata.go` writes the
  data and the notes (`build.Result.Notes`, printed by `xo build`).
  Goldens `time_zones`, `time_civil`, `time_format`, `time_layout_fault`,
  `time_dynamic_zone`; `TestZoneDataPerUse` (what each kind of use
  embeds, `--tzdata=system`); checker golden `time_layout`; habit cases
  `internal/fix/testdata/habits/time-layout-*`. Sizes on darwin/arm64:
  UTC only 7,968,146 bytes, one constant zone 7,968,994, a run time
  name 8,382,498 (std.md 13.5).
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
  programs only (not `xo test`), no `--race`, `--holes` (arenas are ignored),
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
  and Str methods the bench programs use; since 2026-10-08 also core 7
  (decision 0068): `scope`, `spawn`, `spawn_background`, `await`,
  `s.cancel()`, groups from `s.group()`, `task.is_canceled()`, `Chan`
  (send, recv, try_send, try_recv, close, len, `for x in ch`), `select`
  (with `after` since decision 0069), `Mutex` (`with`, `with_read`), and
  `Atomic`. Not yet: `map_concurrent(_until)`, `isolate`, `Semaphore`,
  `Once`, `os.tasks`. `defer` since 2026-10-08 (decision 0070: inlined
  at every exit, shielded; a fault does not run it). Decision 0069 adds
  `os.clock` (`now`, `sleep`, `since`), `task.with_timeout`,
  `task.stop_requested`, `task.stopping`, `os.proc.set_grace`,
  `os.env.get`, Duration (literals, arithmetic, `from_millis`,
  `from_nanos`, `from_secs_int`, `millis`, `nanos`, `seconds`, `abs`) and
  Time (`from_unix`, `from_unix_millis`, `unix`, `unix_millis`) as Int
  nanoseconds, and Bytes as a Str (`Str.bytes`, `to_str`, `len`,
  `is_empty`). Decision 0081 (2026-10-08) adds the sized integers
  (Int8 to UInt64) and Rune as KInt types in the Int word (checked
  arithmetic through `rt.int_op`, UInt64 ordered by flipping the sign
  bit, widening conversions as `iconv`), rune literals as integers,
  `b[i]` on Bytes (`bytes.index`, inline in both backends), slicing of
  Str, Bytes, and List, `b"..."` literals, `StrBuilder` and
  `BytesBuilder` (the text so far, appended in place), the Rune, Str,
  and Bytes text methods of decision 0079 (std/native/prelude.xo),
  `os.fs.read`, `os.env.args`, `os.proc.exit`, and constants of
  collection types built once per run (`rt.const_once`). Enough for
  `selfhost/lexer`.
  Map and Set (decision 0072) are structs of lists with generated
  operations: literals, get, contains, get_or_fault, put, remove,
  update, keys, values, entries, add, to_list, `m[k] = v`, and `for`
  loops; `==` on them since decision 0074; not Float keys, filter, or
  Set algebra.
  `os.net.listen`/`dial` (dial gives up after 30 s like the Go
  backend), `Listener.addr`/`accept`/`close`, and
  `Conn.read`/`write`/`close` (decision 0071) run on the event loop;
  SIGINT/SIGTERM give the two phase shutdown of core 7.5.
  Since decision 0074 (2026-10-08) std is implemented for the native
  backends in Xo: `std/native/<name>.xo` implements `std/<name>`
  (`prelude.xo` the prelude: Str methods, Int and Float parse, the List
  closure methods, sort, Option and Result methods), loaded by the
  checker only for native builds (`check.Options.Native`,
  `build.CheckNative`) and lowered in place of the std declarations
  (`internal/ssa/build_native.go`); `std/native/rt.xo`, `rtjson.xo`,
  and `rthttp.xo` declare the C intrinsics they call (only native
  modules can `use native/...`). So std/http (server, client, routing,
  middleware, request values), std/json (codecs per type,
  build_json.go), and std/log run natively; also interpolation of any
  type (Display, Debug of `derive Display`, Duration, Time,
  build_show.go), `main ! Error` (`error: <text>`, exit 1), `requires`
  clauses, and values of Error, Display, Debug as their text. Values of
  other interface types (decision 0100, 2026-10-09,
  `internal/ssa/build_iface.go`) are `xo.iface[I]` structs of closures
  sharing one environment (`OpRebind`), with hidden `xo.eq`, `xo.tid`,
  `xo.show` fields; goldens `iface_*`. Decision 0101 adds Float32 (a
  double rounded through `rt.f32_round`), `Secret[T]`, `Buf[T]`,
  `xo.cmp[T]` order helpers, and more std value methods.
- TestGenGoldensNative (`internal/llvm/gen_native_test.go`, 2026-10-09)
  builds every internal/gen golden named in
  `internal/llvm/testdata/gen_native.txt` with Go, LLVM, and (on arm64
  hosts) arm64, and requires the same output and no live allocation; add
  a golden there when a gap closes.
- std/native/path.xo (std/path) and std/native/task.xo (Semaphore over a
  channel of permits, Once over a Mutex holding the result,
  map_concurrent over a scope) are native implementations in Xo
  (2026-10-09); goldens `std_path`, `task_native`.
- Fs natively (decision 0102, 2026-10-09): `xo.Fs {root, mode}`, methods
  `fs_*` in std/native/prelude.xo over `std/llrt/xo_fs.c`; standard input
  in `std/llrt/xo_stdin.c` (descriptor 0 on the poller); TestStdinNative
  pipes input into Go and native builds and compares.
- Error frames natively (decision 0104, 2026-10-09): Err carries a hidden
  `xo.tr` trace (internal/ssa build_frames.go); TestErrorTracesNative
  compares whole standard error, frame lines included. The
  differences from the Go backend are listed in decision 0074 (a fault
  in a handler exits; no `http.stream`). HTTPS (std.md 4.6) is
  std/llrt/xo_tls.c on Mbed TLS since 2026-10-09 (decisions 0105, 0106;
  see "TLS library").
- std/time natively (2026-10-09, decision 0094 "Native implementation"):
  std/native/time.xo (civil time, layouts, format, parse) over
  std/llrt/xo_time.c (Go's TZif reader and zone lookup, intrinsics in
  rttime.xo). xo_std.c includes xo_time.c only with `-DXO_TIME`, which
  `timeFlags` (internal/build/tzdata.go) passes only for programs that
  load std/time, together with the generated `xo_tzdata.h` in the build
  directory (per use zone data: mode 0 TZif per constant name, 1 the
  whole zoneinfo.zip, 2 `--tzdata=system`). Tests: TestTimeDiff
  (`XO_TIMEDIFF_OUT=<prefix>` keeps the program and its output),
  TestZoneDataPerUseNative, and the time goldens in gen_native.txt.
  Last verified: 2026-10-09.
- Counts (decision 0058): positive is thread local (plain updates),
  negative is shared (atomic: `atomicrmw`, counts read with `load atomic
  unordered`), INT64_MIN immortal. `OpShare` calls generated
  `xo.share.<T>` helpers that mark a value's heap graph shared. The
  task lowering emits `OpShare` where values cross threads (decision
  0068); `build.Options.ShareProbes` / `llvm.Options.ShareProbes` (tests)
  marks every heap value shared as it is defined (builds go to
  `<build dir>/llvm-share`).
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
  1 GiB stack. The C interface passes only scalars and pointers. Tasks
  (decision 0068, section "Tasks" of xo_rt.c): a pool of worker threads
  (one per CPU, `XO_THREADS=n` overrides), Go style run queues and
  spinning workers, root threads that run queued work while they wait
  (decision 0077; `XO_RTSTATS=1` counts scheduler events,
  `XO_SPIN_ROUNDS=n` tunes spinning, default 16), futex based locks and
  parks (`__ulock_wait` on Darwin), awaits and scope joins that run tasks nobody started, a cancellation
  tree, channels, select, Mutex (readers and writer, writers preferred),
  Atomic, and deadlock detection (XO-F001). Since decision 0069 every task
  runs on a fiber (64 MiB reserved stack with a guard page,
  `XO_TASK_STACK_MB=n` overrides, 64 cached; register switch in top level
  assembly for arm64 and x86_64, ThreadSanitizer fiber API at each
  switch): every wait parks the fiber and frees its worker. A poller
  thread started in `main` (kqueue on Darwin, epoll on Linux, a self
  pipe) runs the timer heap, socket readiness, and SIGINT/SIGTERM (two
  phase shutdown, core 7.5); SIGPIPE is ignored; buffered stdout is
  flushed before any context waits. Thread locals only through
  `noinline` accessors, errno only through `noinline` syscall wrappers
  (`gotchas/fiber-thread-local-address.md`). Generated code calls it as
  `rt.<name>` SSA calls (C function `xo_<name>`), passing values as
  lists of one; handles (Task, Scope, Chan, Mutex, Atomic, Listener,
  Conn) are counted objects created shared. Runtime calls also take and
  return Floats, return a Str (the object, its length through a last
  pointer argument), and `rt.rs_*` calls take their Str operands as
  string objects and consume the first (appending in place). xo_rt.c
  ends by including `xo_std.c` (the native std intrinsics: strings,
  numbers, durations, times, base64), which includes `xo_unicode.c`
  (Go's case ranges and the letter, digit, and `strconv.IsPrint` rune
  ranges, generated: `go test ./internal/llvm -run TestUnicodeTables
  -golden-update`), `xo_bytes.c` (decision 0081: checked arithmetic and
  conversions of sized integers, Rune text and quoting, Bytes slicing and
  decoding, the builders' appends, `fs.read`, `env.args`, and the slots
  of constants of collection types), `xo_json.c`, and `xo_http.c`;
  builds write every `std/llrt/*.c` next to xo_rt.c and compile only
  xo_rt.c.
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
  recursion ones, `gotchas/tsan-deep-recursion.md`); TestRuntimeTasks
  (`std/llrt/testdata/tasks.c`, the task runtime driven from C, under
  ThreadSanitizer with one and three workers, then `-O2 -DN=20000`),
  TestRuntimeEventLoop (`std/llrt/testdata/evloop.c`: 2000 sleepers on
  few workers, an 8 MiB echo through a socket, close and cancellation
  waking parked socket operations, a refused dial, select `after`; same
  runs), TestGoldenTasks (the goldens that use tasks, built with
  ThreadSanitizer without probes and run with `XO_THREADS` 1, 2, and
  the default against want.txt), TestDeadlock (internal/gen's
  `deadlock_real`, read only, natively: XO-F001); they skip when clang
  has no ThreadSanitizer. The `conc_*` goldens are concurrency programs
  adapted from internal/gen's goldens without clock, I/O, Map, or
  Display, so the Go, LLVM, and arm64 backends print the same. TestUnsupported checks the error
  for constructs outside the subset (`testdata/unsupported/`).
  `cmd/xo/llvm_test.go` drives the CLI.

## arm64 backend (native)

Last verified: 2026-10-08. Decisions 0049, 0051, and 0054; results
`bench/results/arm64-native-2026-10-08.md` and
`bench/results/arm64-perf-2026-10-08.md`.

- `xo build --backend=arm64` and `xo run --backend=arm64`; programs only,
  no `--race`, `--holes` (arenas are ignored); targets darwin/arm64 and
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
- Unwinding (2026-10-09, decision 0085): every function has `.cfi_*`
  call frame information (leaf functions' fault stubs push a frame record
  first); a function with landing pads (`inst.pad`) gets
  `.cfi_personality 155, _xo_personality`, `.cfi_lsda`, and a call site
  table in `__gcc_except_tab` / `.gcc_except_table` labelled
  `GCC_except_table_xoN` on Mach-O (a symbol: see
  gotchas/arm64-lsda-local-label.md); call sites are `fN_csKa`..`fN_csKb`
  around each `bl`. A check only a fault takes is an `iJump` with the pad
  as a second target (liveness and reachability only); values live into a
  pad entered from a fault stub avoid caller saved registers.
- Tests: `go test ./internal/arm64` (golden assembly; on an arm64 host
  it builds and runs every golden of internal/llvm against its want.txt
  (62 on 2026-10-08, with the native_* and http_* goldens of decision 0074), the bench programs, and `testdata/run/*` against the Go
  backend, all with the leak check; TestGoldenRunShared runs the goldens
  with share probes and assembles their Linux output as ELF). To try a code change by hand, edit
  `<build dir>/arm64/prog-darwin.s` and link it with
  `cc -O2 prog-darwin.s xo_rt.c` (add `-isysroot <Xcode SDK>` when the
  default SDK does not link; on 2026-10-08 the default MacOSX27.0 SDK
  failed with "tapi error: unknown architecture arm64e.x1" and
  `-isysroot /Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk`
  linked). On linux/arm64 the same tests link ELF and run every golden
  (68 on 2026-10-09, `scripts/linux-test.sh arm64`). `XO_BUILD_DIR=<dir>` keeps the build directory (and the `.s`)
  where you want it. `regalloc` fails with "reads virtual register n
  before it is written" when MIR uses a register no path defines
  (`gotchas/arm64-mir-param-without-moves.md`).

## Hybrid builds (Go backend with Go assembler)

Last verified: 2026-10-09. Decision 0084; results
`bench/results/go-asm-hybrid-2026-10-09.md`.

- `xo build --hybrid` (Go backend, Build mode): `internal/build/hybrid.go`
  runs the native check, SSA with reference counting, and
  `arm64.GenerateGo` (Go mode, `internal/arm64/goasm.go`); a program
  outside the SSA subset builds as plain Go (`HybridInfo.Err`). The CLI
  prints `hybrid: f, g in Go assembler` on stderr. `build.HybridPlan`
  reports the plan without building; `Options.HybridAll` takes every
  eligible function (tests), `Options.HybridPure` builds the same
  directory with tag `xo_pure`.
- Eligible: free functions of the main module (no methods, generic
  instances, `ensures` clauses, structs with invariants); parameters and
  results Int, Float, Bool, structs of Int and Float fields, Lists of
  those (and of Bool); `var` only on Lists; no allocation, no reference
  count, no Str; calls only to eligible functions and the inline list
  and scalar methods (`goMethods`). Selected (wrapped): eligible with a
  loop. Rejection reasons are in `GoProgram.Rejected`.
- Generated files in the main module's package: `xo.go` (the rest),
  `xo_pure.go` (`//go:build !arm64 || xo_pure`, the Go versions),
  `xo_hybrid.go` (`//go:build arm64 && !xo_pure`: ABI0 declarations
  `func xoasm_f(p0 []Rec, p1 int64) (r0 int64)`, `//go:noescape` when the
  function calls no Xo function, wrappers that `Unshared()` written `var`
  Lists and pass `.Slice()`, `xoHybridFault` and the position table),
  `xo_hybrid_arm64.s` (`//go:build !xo_pure`). Imports a file does not
  name are left out of it (`usesIdent`).
- Go mode differences from the native target: List word is the slice
  base (`hdr()` 0), lengths are parameter registers (`goLens`); arguments
  in memory (`name+off(FP)`, outgoing at 8(RSP)); registers x0 to x14,
  x19 to x25, d0 to d28 all caller saved (x18, x26 to x30 untouched,
  x15 to x17 and d29 to d31 the emitter's scratch); values live across a
  call keep their register and are stored and reloaded around it
  (`inst.saves`); frame `$outArgs+8*slots`, NOSPLIT unless the function
  calls an Xo function, `NO_LOCAL_POINTERS` when it calls anything; no
  jump tables; faults `CALL ·xoHybridFault(SB)` with (kind, a, b, c, pos),
  kinds in `goFaultKind`, raised by `rt.HybridFault`.
- `gosyntax.go` translates the GNU text line by line; `TestGoSyntaxEncoding`
  (darwin/arm64 with cc) assembles every emitted form both ways and
  compares the machine code (Go writes `mov x, #0` as `orr`).
- Tests: `go test ./internal/gen -run 'TestGoldenHybrid|TestHybridRandom'`
  (eligible goldens against want.txt, 12 random kernel programs (3 with
  `-short`) against their `xo_pure` build, all four fault kinds; `go vet
  -asmdecl` on each build directory). Use `-asmdecl` only: the generated
  Go has unreachable code that plain `go vet` reports.
- Profile a hybrid binary with `XO_CPUPROFILE=p.prof ./bin` and
  `go tool pprof -top p.prof` (assembly functions appear as
  `xoasm_<name>`).

## TLS library (native std/http, decisions 0105, 0106, 0117)

Last verified: 2026-10-10.

- Library: Mbed TLS 4.1.1 (LTS, maintained until March 2029), with
  TF-PSA-Crypto, Apache-2.0. Pinned release tarball
  `https://github.com/Mbed-TLS/mbedtls/releases/download/mbedtls-4.1.1/mbedtls-4.1.1.tar.bz2`,
  SHA-256 `3359a349e23db3d5536fcee032ae7b2ecbfc08972fab643089b5cbf2a375c98c`
  (the release's own `mbedtls-4.1.1-sha256sum.txt` agrees).
  `third_party/mbedtls/src/` holds its `.c` and `.h` files the runtime
  compiles, unmodified (the tarball ships the generated sources, so no
  Perl or Python is needed); `xo/xo_tls_config.h` and
  `xo/xo_crypto_config.h` are the Xo configuration (TLS 1.2 and 1.3,
  ECDHE, server session tickets, no DTLS, PSK, session cache,
  renegotiation, debug, or file I/O; threading on; SHA-256 and SHA-512
  instructions when the CPU has them). `xo/xo_tls_accel.c` is compiled
  into the archive and replaces library functions without patching them
  (decision 0117): `mbedtls.FileDefines` gives single library files extra
  flags, `-D<fn>=xo_builtin_<fn>` renames the original, and the Xo file
  defines the name (X25519 public keys through HACL*, P-256 inverses by
  Fermat, a spinning lock for the library's mutexes). `mbedtls.tgz` is `src/` compressed (1.3 MB; the xo
  binary embeds it instead of the 6.5 MB of sources), made by
  `go generate ./third_party/mbedtls`; TestArchive fails when it does
  not match `src/`.
- Update: `scripts/vendor-mbedtls.sh <version> <sha256>`, then set
  `Version` in `third_party/mbedtls/mbedtls.go`, the pinned values in the
  script and here, and run `go test ./third_party/mbedtls ./internal/build`
  and `go test ./internal/llvm -run 'TestTLSDiff|TestGolden.*/http_tls'`.
- Build (internal/build/tls.go): a native link whose generated code names
  a std/http runtime function (`xo_cl_do`, `xo_srv_new`, `xo_tls_*`) is
  compiled with `-DXO_TLS`, the library's include directories and
  configuration defines, and linked with `libxotls.a` (plus `-framework
  Security -framework CoreFoundation` on macOS, for the system's roots).
  The archive is built on first use per C compiler, compiler version,
  sysroot, host, and sanitizer flags (`-fsanitize=...` of the link) into
  `<user cache>/xo/mbedtls/4.1.1-<key>/` (`$XO_TLS_CACHE` overrides the
  root): the tree is extracted, every library `.c` compiled with
  `-O2 -fexceptions -funwind-tables` in parallel (about 2 to 3 s), and
  archived with `ar` (`$XO_AR`, else `ar`, else `llvm-ar`). Other
  programs compile xo_tls.c's stubs and link nothing of it.
- Native builds are host only (the arm64 backend writes other arm64
  targets as assembly only), so the library is compiled by the same
  clang as the runtime on each OS: darwin/arm64, linux/arm64, and
  linux/amd64 (the containers of "Linux (containers)") build and pass
  the TLS tests. Nothing in it is architecture specific beyond what
  Mbed TLS detects itself (AES-NI on x86-64, the ARMv8 crypto extension
  on arm64).
- Handshake load: `SCHEME=https NEWCONN=1 [RESUME=1] CONFIGS="xo xo-llvm"
  bench/perf/http/run.sh` (a full, or with RESUME=1 a resumed, handshake
  per request); profile the server with `sample <pid> 4` (2026-10-10:
  15k full handshakes/s native against 17k on the Go backend,
  `bench/results/native-runtime-2026-10-10.md`).
- Sizes (stripped, darwin/arm64, 2026-10-09): a std/http program grows
  by about 250 KB with the LLVM backend (336 to 582 KB) and 340 KB with
  the arm64 backend (375 to 711 KB); an HTTPS server and client program is
  599 KB (LLVM) and 711 KB (arm64), against 7.8 MB on the Go backend.
- Tests: `go test ./internal/build -run TestTLSLibrary` (builds the
  archive into a temporary cache and links a C program with it);
  `go test ./internal/build -run TestTLSAccel` (testdata/tls_accel.c:
  xo_tls_accel.c against RFC 6979 and RFC 7748 answers and against the
  renamed library functions on random inputs);
  `go test ./internal/llvm -run TestTLSDiff` (about 15 s: an HTTPS program
  with the Go, LLVM, and arm64 backends against Go peers, certificates made
  in `t.TempDir()`); `TestTLSLeaks` (Linux only: the `http_tls` golden
  under AddressSanitizer with LeakSanitizer on, passes on linux/arm64
  2026-10-09); the `http_tls` golden (committed test only
  certificates valid to 2120, plus an expired and a not yet valid one).

## Linux (containers)

Last verified: 2026-10-09. Results: `bench/results/linux-2026-10-09.md`.

- `scripts/linux-test.sh [arm64|amd64] [go test arguments...]` builds the
  image `xo-linux-test:<arch>` from `bench/linux/Dockerfile`
  (`golang:<host Go version>-trixie` plus Debian's clang 19, lld, and the
  compiler-rt sanitizer runtimes) and runs the tests in a container that
  is removed afterwards. The repository is mounted read only at `/src`;
  the Go cache, `HOME` (so the xo build directory), and `TMPDIR` live in
  the Docker volume `xo-linux-cache-<arch>` (`docker volume rm` it to
  start cold). `GOFLAGS=-buildvcs=false` because the worktree's `.git`
  points outside the mount.
- Default arm64 run: `go vet` then `go test -count=1 -p 2` over
  `go list -e ./... | grep -v /runs/`
  (about 6 min on an M series Mac): the Go backend on Linux, the LLVM and
  arm64 backends' goldens linked as ELF and run (epoll poller, futex
  parks, fibers, SIGTERM, TCP, HTTP), ThreadSanitizer, AddressSanitizer,
  and UBSan.
- Default amd64 run (emulated: Docker Desktop uses Rosetta, `/proc/cpuinfo`
  says `VirtualApple`): `go test -short -p 2 ./...` then the full
  `./internal/llvm` (x86-64 LLVM IR, the x86_64 fiber switch, epoll).
  ThreadSanitizer cannot start there; the TSan tests run their programs
  without it (`gotchas/tsan-under-x86-emulation.md`). The arm64 backend
  only emits assembly on amd64. The full amd64 suite also passes but
  needs `-p 1 -parallel 4` for internal/gen and internal/llvm (about
  4 and 5 min): translated compilers use more memory.
- Extra arguments replace the default, for example
  `scripts/linux-test.sh arm64 -count=1 -p 1 -run 'ASan|HTTP' ./internal/llvm`.
  Keep `-p` low: the Docker VM (8 GiB on 2026-10-09) runs out of memory
  with parallel Go builds of the generated runtime
  (`gotchas/docker-vm-memory-go-compile-killed.md`).
- Requires Docker; the container gets `SYS_PTRACE` and no seccomp
  profile (sanitizers). Containers are named `xo-linux-<arch>-<pid>`.

## Linux (EC2, infra/ci-aws)

Last verified: 2026-10-09. Results: `bench/results/ci-aws-2026-10-09.md`.

- `infra/ci-aws/ci.sh [short|full|race|sanitizers ...]` runs the CI jobs on
  real Linux machines: Terraform (local state in `infra/ci-aws/`) creates
  one key pair, one security group (tcp 22 from 203.0.113.10/32 only),
  and a `c7g.4xlarge` (arm64) and a `c7i.4xlarge` (amd64) with Ubuntu
  24.04 (Canonical's AMI by data source), all tagged `project=xo-ci`, in
  AWS account ACCOUNT_ID only (the provider's `allowed_account_ids`).
- It rsyncs the working tree (no `.git/`, `runs/`, `.tools/`, `.claude/`),
  installs the host's Go version from go.dev plus clang 18, lld, llvm,
  build-essential, and `libclang-rt-18-dev`
  (`gotchas/ubuntu-clang-without-sanitizer-runtimes.md`), runs the jobs on
  both machines in parallel, each job with a cold `GOCACHE`, and copies
  the logs to `infra/ci-aws/logs/<timestamp>/<arch>/` (gitignored).
- An EXIT trap always runs `terraform destroy -auto-approve`, deletes the
  generated key `infra/ci-aws/xo-ci-key.pem`, and runs
  `infra/ci-aws/verify-teardown.sh` (instances, key pair, security group,
  volumes: every list must be `[]`). By hand:
  `terraform -chdir=infra/ci-aws destroy -auto-approve`.
- A whole run takes about 20 minutes and costs about $0.45 on demand
  (2026-10-09). TSan runs natively on both (unlike amd64 under Rosetta).
- Needs terraform (1.16.2 used), the AWS CLI, rsync, ssh, python3.

## Modules and workspaces (decision 0086, steps 1 to 4)

Last verified: 2026-10-09.

- Package `internal/mod`: `modfile.go` (xo.mod `require <path> <version>`
  parsing, `SetRequire`, semver `Compare`), `cache.go` (module cache, git
  fetch, `RepoURL`), `lock.go` (`HashDir`, `HashModFile`, xo.lock read and
  write), `load.go` (`Load`: MVS, verification, vendor mode, workspace),
  `work.go` (xo.work, `WorkspaceDirs`), `cmds.go` (`Get`, `Tidy`,
  `Vendor`). `internal/mod/modtest` builds local git repositories for
  tests from `internal/mod/testdata/repos/<module path>@<version>/`.
- Module cache: `$XO_MODCACHE`, else `<user cache dir>/xo/mod` (macOS
  `~/Library/Caches/xo/mod`, Linux `~/.cache/xo/mod`). A version is
  extracted to `<cache>/<escaped path>@<version>` (upper case letters
  escaped as `!` plus lower case) and made read only (files 0444, dirs
  0555); bare clones live in `<cache>/cache/vcs/<sha256 of URL>`. `xo mod
  clean` removes the cache (plain `rm -rf` fails on the read only dirs).
- Fetch: `git clone --bare --no-local`, `git fetch` of tags when a tag is
  missing, `git archive refs/tags/<v>` extracted in Go (no symlinks, no
  paths leaving the tree). git runs with `core.hooksPath=/dev/null`,
  `GIT_TERMINAL_PROMPT=0`, `GIT_CONFIG_NOSYSTEM=1`. The module must have
  its xo.mod in its directory declaring the same path. Directories below
  it that hold an xo.mod (nested modules) are removed after extraction,
  and `HashDir` skips them (a vendored root module holds its vendored
  subdirectory modules). Last verified: 2026-10-09.
- Locations: `github.com/a/b` is `https://github.com/a/b.git` (also
  gitlab.com, bitbucket.org), any other path `https://<path>`.
  `XO_GIT_BASE=<dir or URL>` maps every path to `<base>/<path>`;
  `XO_GIT_REWRITE=<prefix>=<url>,...` rewrites by longest prefix (wins
  over XO_GIT_BASE).
- Subdirectory modules (`internal/mod/repo.go` `Locate`): on the three
  known hosts the repository is the first three path elements and the
  rest (without `/vN`) is the directory; elsewhere `git ls-remote <url>
  HEAD` is tried for each prefix, longest first (a cached clone of the
  whole path skips it), memoized per process by git location. Versions
  are tags `<dir>/vX.Y.Z`; the tree is `git archive
  refs/tags/<dir>/<v>:<dir>`. `xo publish` finds the directory from `git
  rev-parse --show-toplevel`, requires the module path (without `/vN`) to
  end in it, lists `<dir>/v*` tags (ignoring majors above the path's), and
  prints `git tag <dir>/vX.Y.Z`. Last verified: 2026-10-09.
- MVS: walk every reachable `path version` from the main module (or every
  workspace module), keep the highest version per path. Every visited
  version is downloaded (its xo.mod is read from the cache tree).
- xo.lock: `<path> <version> h1:<tree hash>` for selected versions and
  `<path> <version>/xo.mod h1:<hash>` for every version read. Hash: base64
  SHA-256 of sorted `"<sha256 hex>  <slash path>\n"` lines (Go's h1
  shape over the raw tree). Every check and build rehashes the trees and
  compares (XO1304 mismatch, XO1305 missing). Only `xo get` and `xo mod
  tidy` write it, always without workspace and vendor.
- Vendor: `xo vendor` copies selected trees to `vendor/<path>/` (writable
  copies) and writes `vendor/modules.txt` (`# <path> <version>` lines).
  When `vendor/modules.txt` exists (and no workspace is active), builds
  read only vendor/, check each direct require is vendored at its version
  or above (XO1307), and verify tree hashes against xo.lock. The checker
  skips a `vendor/` directory holding `modules.txt` when walking dirs.
- Offline: `XO_OFFLINE=1` or `--offline` (any command; removed from the
  arguments before dispatch in `cmd/xo/mod.go` `moduleFlags`, env
  restored after the command) never runs git; a missing version is
  XO1303; `xo get` resolves `latest` from the cached versions.
- Workspaces: `xo.work` (`xo 2026`, `use <dir>` lines) found at or above
  the main module, or `XO_WORK=<file>`; `XO_WORK=off` or `--workspace=off`
  ignores it. It applies only when it uses the main module. Used modules
  override requirements (their requirements join MVS; xo.lock is the
  union of their locks for verification). `query.Load` (check, query,
  mcp), `xo edit`, and `xo test` add every workspace module's directory
  (`mod.WorkspaceDirs`); `xo doc` uses `query.LoadExact`. `xo build --release`
  and `xo build --ci` fail with XO1308 naming the overridden modules. Build info records
  `deps` (path to version, or `workspace <dir>`).
- Checker integration (`internal/check/deps.go`): `loadDirs` calls
  `mod.Load` once per main module and registers each selected module's
  directory as a root; imports resolve by the longest module path prefix.
  Problems become diagnostics on the xo.mod/xo.lock/xo.work line;
  imports of a module with a problem are not reported again. XO1306: a
  remote import no root covers, or a package of a module that is in the
  build only because another module requires it. Modules from the cache
  or vendor/ have `Module.Dependency` set; `xo edit` refuses to change
  them. `use go` in a dependency binds into the main module's cache.
- `xo serve` stamps xo.lock, vendor/modules.txt, and the nearest xo.work
  of each module root (used or not) and records which file that is
  (`mod.NearestWork`), so a new, removed, or edited workspace above the
  directories is noticed; `serve.Forward` checks in process when
  XO_OFFLINE, XO_WORK, XO_MODCACHE, XO_GIT_BASE, or XO_GIT_REWRITE is set.
- Effect ceilings (step 3): `internal/mod/ceiling.go` (`Ceiling`,
  `ParseCeiling`, `Ceilings(dir)`: xo.mod requires plus the workspace's
  `use ... uses` lines), `internal/check/ceiling.go` (`checkCeiling`,
  called from `callFunc` and `funcValue`; XO1309 with a note on the
  declaring line). `SetRequire` keeps a ceiling when `xo get` or `xo mod
  tidy` rewrites the version. `mod.EffectLabels` is kept equal to the
  checker's labels by `TestCeilingLabels`.
- API diffs and publish (step 4): package `internal/release`
  (`ModuleAPI`: the syntax level `xo query api` surface of every package
  via `query.DeclSurface`; `Diff`, `Bump`; `Publish`). `xo get` prints the
  diff of every changed requirement (JSON `api`), `xo get -u` upgrades
  every requirement; `xo publish [--check] [--version v] [--json]`
  (`cmd/xo/publish.go`) reads tags with `mod.LocalTags`, extracts the
  previous tag with `mod.ExtractTag` into a temp dir, and prints the tag
  command (never runs git tag or push). `xo init <path>` writes xo.mod and
  `/xo.work` in .gitignore.
- Major versions: `internal/mod/major.go` (`MajorSuffix`,
  `MajorMismatch`); `RepoURL` strips a `/vN` suffix; `Versions` keeps the
  tags of the path's major; `localName` (`internal/check/load.go`) skips
  the suffix.
- Tests: `internal/mod/mod_test.go` (MVS, lock goldens
  `testdata/golden/*.lock`, moved tag, fetch errors, offline, get, tidy,
  vendor, workspace), `internal/mod/ceiling_test.go` (ceilings, /vN),
  `cmd/xo/mod_test.go` (`TestModules`: the commands end to end; runs the
  built program unless -short), `cmd/xo/release_test.go`
  (`TestEffectCeilings`, `TestPublish`, `TestInit`),
  `cmd/xo/subdir_test.go` (`TestSubdirModules`: tidy, lock, run, get,
  vendor, cli/v2, publish in a subdirectory), `internal/mod/repo_test.go`
  (`TestLocate`: probing on another host, nested module pruning),
  `internal/release/api_test.go`, `internal/serve/serve_test.go`,
  `internal/check/rules_test.go` (XO1306), `internal/check/ceiling_test.go`
  (XO1309). No test touches the network.
- Not yet: per definition hashes (step 5), a proxy or checksum log,
  Go's "major subdirectory" layout (`/v2/` directories of one tag; major
  versions of a subdirectory module use the major branch way, tags
  `<dir>/v2.x.y`), `go-get` style HTML meta discovery of repository
  roots, `xo get --uses` to set a ceiling from the command line.

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
  `xo-bind: v<N>` (gobind.Version; 2 since 2026-10-08, when callbacks
  stopped carrying the request's arena, decision 0066; 3 since
  2026-10-08, decision 0067: adapters call `GoEnter(tk)` and defer
  `GoExit(tk, name)`, and callbacks go through `rt.NewGoCallback(tk)`
  with `cb.Begin()`/`cb.End(ctk)` per call; 4 since 2026-10-08, decision
  0073: callback arguments are converted first and passed as
  `cb.Begin(func() []any {...})`, and `*time.Timer`/`*time.Ticker` are
  handles, so `time.after_func` binds; 5 since 2026-10-09, decision 0093:
  adapters call `rt.GoFxValue`/`rt.GoFxRerun` when `rt.GoFxOn()`, by
  value calls live in `goCall_<prefix><name>`, declarations carry a
  `/// Replay:` line) or another module version
  than the pin, or another `replay rerun go` list (header line
  `// replay-rerun: <names>`). Pins: `require go <module> <version>`
  lines in xo.mod. A binding made with a rerun list is cached as
  `<cache>/<path>.rerun-<8 hex>.xo` and `.go`, beside the plain one.
  Last verified: 2026-10-09.
- Checker (`internal/check/load.go` `loadGoImport`): binds on demand, loads
  the declaration file as a module `go:<path>` with `IsStd` set and
  `Module.Go` holding the binding. XO1101 when binding fails, XO1102 when
  a name or `Type.method` is in the binding's skip list. Effect label `go`
  (effects.go).
- Codegen: `stdPrefix` returns the binding prefix, so calls lower as std
  calls. `gen.Generate` adds `rt/xo_gobind_<path>.go` per binding and
  writes the workspace's `require` lines into the build `go.mod` and its
  `go.sum`. Runtime helpers: `std/rt/gointerop.go` (`Go_Error`,
  `GoEnter`/`GoExit` around each Go call (panic to fault, `thread.inGo`),
  `GoBoundary` for field accessors, `GoCallback` (a call on another
  goroutine is a child task with its own thread, decision 0067),
  `GoContext`, `GoListOf`/`GoSliceOf`/`GoMapOf`/`GoMapToGo` copies,
  `GoCollect` for iterators), `std/rt/godeadlock.go` (XO-F001 through Go
  calls waiting for stuck callbacks, decision 0073), `std/rt/gosched.go`
  (foreign callbacks admitted at settled decisions and recorded, decision
  0073). `curG` (goroutine identity) is assembly in
  `std/rt/curg_amd64.s` and `curg_arm64.s` (the g register or TLS) with a
  `runtime.Stack` fallback in `curg_other.go`; `std/embed.go` embeds
  `rt/*.go rt/*.s`, so a new `.s` file in std/rt is copied into build
  directories. Last verified: 2026-10-08.
- Tests: `internal/gobind` (names, pins, mapping, cache staleness),
  `internal/check/rules_go_test.go`, `internal/gen/gobind_test.go`
  (`TestGoBindingSignatures`: 20 std packages, every declaration's Go
  signature as gen calls it equals the adapter's), goldens `go_*` and
  `test_go_interop`, `test_go_callback_deadlock` and
  `go_callback_deadlock_real` (decision 0073: a Go call waiting for a
  stuck callback is XO-F001, a sleeping one is not), `test_go_callback_task` (`-explore 40`, also under
  -race: a callback on Go's goroutine holds mutexes beside a test task and
  a request), `cmd/xo/bind_test.go`, `std/rt/gointerop_test.go`,
  `std/rt/gocallback_test.go`, `internal/gen/replay_gocallback_test.go`,
  `internal/gen/replay_gocalls_test.go` and `internal/gobind/replay_test.go`
  (Go calls recorded and replayed, decision 0093)
  (`TestReplayGoCallback`: seeds reproduce foreign callbacks, recordings
  replay them; runs under -short too).
  The golden `go_uuid` needs `github.com/google/uuid@v1.6.0` from the
  network or module cache and is skipped when `go mod download` fails. The
  golden and checker tests set `XO_GOBIND_DIR` to a temporary directory.

## C interop (`use c`, native builds)

Last verified: 2026-10-10. Decision 0116 (stage 2 of decision 0093);
spec core 4.5.

- Syntax: `use c "<header>" { link "<x>" } [as name] [uses none | uses
  e, ...]` (`ast.UseDecl.C`, `Links`, `Uses`; the lexer reads `c` as an
  import path like `go`). `xo fmt` keeps the clauses in that order.
- Binding (`internal/cbind`, `xo bind --c [--dir d] [--link x]...
  [--uses none|e,...] <header>...`): writes `#include "<abs header>"`
  (module header) or `#include <h>` (include path) into a temporary
  file, runs `clang -fsyntax-only -Xclang -ast-dump=json -x c` on it
  (`$XO_CLANG`, then `clang`; on macOS a failure is retried with
  `-isysroot $(xcrun --show-sdk-path)`), decodes the dump keeping key
  order, and binds the functions whose location is in the header.
  Writes `<cache>/c_<name>_<8 hex>.xo` (declarations, signature mode)
  and `.c` (the adapter, for reading; builds take it from memory). The
  hex is of the header path, links, and effects, so two `uses` ceilings
  of one header are two modules. Within one process a binding is reused
  while the header's contents are the same.
- Cache: `$XO_CBIND_DIR`, else `<xo.mod dir or module dir>/.xo/cbind`.
- Checker (`internal/check/cimport.go` `loadCImport`): XO1201 when the
  check is not native (`Options.Native` false: `xo check` without
  `--no-gc`, the Go backend); XO1104 when binding fails or a `uses`
  label is unknown; XO1105 for a skipped name; the module `c:<...>` is
  `IsStd` with `Module.C` set. C handles are handles for decision 0112.
- SSA (`internal/ssa/build_cffi.go`): a call becomes `rt.<prefix><fn>`
  (the C symbol `xo_<prefix><fn>`); a C handle type lowers to KInt (the
  pointer, not reference counted); a struct argument is passed field by
  field; a struct result is read through `<sym>_r<i>` getters; a handle
  result branches on NULL into `T?`.
- Build (`internal/build/cffi.go`): adapters go to `<build>/cffi/`, link
  sources and archives are added by absolute path, `-l` flags go after
  the objects (`ccBuild` splits them). No `-I` (it would turn off the
  runtime object cache): adapters include the header by absolute path.
  A build with module C files is not binary cached.
- Runtime: `xo_cffi_str` (std/llrt/xo_std.c) makes the Str results.
- Tests: internal/cbind (tiny.h mapping and skips, zlib.h when present,
  the adapter compiles with -Wall -Werror), internal/check
  `TestUseC`, cmd/xo `TestBindC`, internal/llvm `TestUseC` (archive
  built in the test with clang and ar, and C source; arm64 and LLVM,
  leak check) and `TestUseCZlib` (skips without zlib.h).

## Runtime (std/rt)

Last verified: 2026-10-08.

- `core.go` Option/Result/tuples/Box; `frames.go` error frames (2026-10-09,
  decision 0092): `Trace` with `Frame0` to `Frame6` (frame and values in
  one allocation) and `FrameN`, formatted only by `Trace.Lines`;
  `debugBounded` stops `Debug` formatting at a limit (`debugTo` checks a
  registry of bounded builders, `boundedActive`; `writeQuoted` quotes
  long strings only as far as needed); `FrameTextOf` for values formatted
  at capture; `arith.go` checked integer
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
  `SubIUp`, `SubIDown`, `MulIK`, `MulKI`, `CmpO`, `List_AtM`, `List_AtU`;
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
- `arena.go` (2026-10-08): request scoped arenas (default, decision 0066). The HTTP
  server takes an `Arena` from a bounded channel free list (128 kept; not
  a sync.Pool, `gotchas/sync-pool-drops-on-gc.md`) per request, puts it on
  the request `Task` (`Task.arena`; scope bodies share it, spawned tasks
  never get it), and resets it after the response is written. It holds
  the request's header, parameter, and query Map tables, the body bytes,
  an arena response's JSON bytes and header table, and typed bump slabs
  for collection temporaries; buffers over 64 KB are not kept. Routes not
  marked confined, and every route behind middleware, get `heapCopy` of
  the request; a mounted router's middleware and `r.call` run with arena
  responses off (`noArenaResp`); the recorder gets heap copies of bodies
  (`recCopyReq`, `recCopyResp`); Go callbacks run with
  `GoCallbackTask` (no arena). Soundness argument: the comment on
  `Arena`; audit table: decision 0066. `world.go`: the virtual clock of tests (it
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
  test's own child by PID). `TestSignalStopsReadLine` and
  `TestCancelStopsReadLine` (2026-10-09, decision 0088) run it with
  `STDIN=pipe` so standard input stays open and `read_line` blocks.
- `stdin.go` (2026-10-09, decision 0088): stoppable reads of the real
  standard input through one reader goroutine; golden `test_stdin_stop`
  covers the fake standard input and `sleep_or_cancel` on the fake clock,
  golden `codex_gaps` (Go and native: internal/llvm/testdata/golden)
  covers `Duration.parse`, tuple JSON, and `sleep_or_cancel`.
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
| `check` | `dirs`, `no_gc`, `fix` (decision 0096: apply the machine fixes first) | `{ok, errors, warnings, modules, diagnostics}` (diagnostics with notes and fixes, core 10.2), plus `applied` with `fix` |
| `query` | `kind` (type, def, callers, effects, holes, api, imports, deps), `arg`, `dir` | the `xo query --json` answer |
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

- Self hosted lexer (2026-10-08, decisions 0076 and 0081): `selfhost/lexer`
  builds on the Go, arm64, and LLVM backends, and `TestSelfhostDifferential`
  compares all three with the Go lexer (the native ones skip without
  clang, and arm64 off arm64 hosts). The Xo tests of the module
  (`xo test`) run on the Go backend only: the native backends build
  programs, not tests. A change to `internal/lexer` or a lexical
  diagnostic must be ported to `selfhost/lexer` in the same change.
  Remaining gaps with proposed fixes:
  `bench/results/selfhost-lexer-2026-10-08.md`. Last verified: 2026-10-08.

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
  and 68 with `--arenas`) against Go's 3.0 KB and 31 (later, after other
  allocation cuts: 9.5 KB and 77 with `--no-arenas`, 5.5 KB and 49 with
  arenas, the default; decision 0066); throughput is 0.43x
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

Per program growth (2026-10-09, after the fixes of
`gotchas/go-generic-method-sets-per-package.md`): hello world 2.9 MB,
examples/notes-service 6.9 MB, its test binary 8.4 MB (34, 54, and 68 MB
before); the runtime itself is about 13 MB per variant (with and without
`xo_nohttp`). Last verified: 2026-10-09.

Xo builds and bindings run Go with `GOCACHE` set to `$XO_GOCACHE` (default
`<user cache>/xo/gocache`), trimmed by least recent use to stay under
`XO_GOCACHE_MAX` bytes (default 8 GiB). Entries younger than 2 hours are never trimmed, so concurrent builds keep their files. See `gotchas/go-build-cache-growth.md` and `gotchas/go-cache-trim-races-builds.md`.

