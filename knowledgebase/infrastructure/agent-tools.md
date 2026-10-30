# Agent tools: what exists, and when to reach for it

One index of the tools built for agents working on Xo (owner,
2026-10-18: make sure every agent knows them). Each entry names where
its full documentation lives. Last verified: 2026-10-18 (main 41a6afe2).

## Finding code (decision 0261)

- `xo def NAME`, `xo refs NAME`, `xo type FILE:LINE:COL`, `xo doc NAME`
  (a type's methods), `xo symbols [DIR] [--pub]` (the directory's own
  declarations; at a root with no .xo files of its own it names the
  directories below that have them; `--file=F.xo` or a .xo argument for
  one file, `--module=std/json` or `native/llrt/pool` or an import path
  for one module, 2026-10-21); `--dir=DIR`. Output is compact text, a
  record a line, everywhere (decision 0386, 2026-10-26: the old JSON
  default printed `xo symbols --dir=compiler/ssa` as one 540 KB line):
  symbols `kind name  file:line  signature`, refs `file:line:col
  context`; `xo symbols [--dir=D] NAME` filters by name (substring, or
  `check_*`), `--limit=N` on symbols and refs, `--doc` adds symbols' doc
  comments, `--json` is JSON a record a line for scripts. Pipe through
  grep or head freely; prefer NAME and --limit over a whole listing.
  Use them instead of grep for declarations, callers, types, and to check
  that a method exists before calling it (`Map.update` existed while
  agents guessed `update_or`, 2026-10-18).
- Names (decision 0289, the same in def, refs, doc, and `xo query`): write
  them as code does: `check.Checker.check_dirs`, `Str.find`, `j.f` for
  `use std/json as j`, `main.f`, `std/tls.client`; std and runtime
  modules load when named (`tls.client`, `pool.park_wait`), so
  `xo refs pool.park_wait` finds tasks.xo's call too. A runtime name is
  qualified by its file (`pool.park_wait` in std/native/llrt/pool.xo);
  refs lists the calls inside std and the runtime (their bodies are
  checked for it), from a program's directory, `--dir=std/native`, or
  `--dir=std/native/llrt` (each runtime file its own module there, so a
  bare `park_wait` lists pool's, poll's, and tasks'). A miss suggests the
  std functions of that name (`nosuchmod.park_wait` -> `pool.park_wait`).
  `--dir` may be a workspace or repository root; a module there whose
  `use go` or `use c` binding is missing (the xo checkout's
  examples/realtime-control), or whose requirement is not in the module
  cache, is left out and the rest checked, and the answer names it
  (`"skipped"` in JSON, a `note: skipped` line in text).
- A root's check skips every `testdata` directory, as each DIR/... walk
  does (decision 0210), so the program goldens under
  internal/*/testdata are not searched: refs names the directories it
  left out (`"testdata_skipped"`, a `note: not checked` line), and
  `xo refs --testdata` checks them too. At a root, refs checks only the
  modules whose text spells the name's last segment as a whole
  identifier (a module that does not cannot refer to it; 2026-10-26):
  inside those, a function body that spells none of the names is not
  parsed or checked (Checker.body_words). At the xo root with
  --testdata, `trim_prefix` (350 refs) takes 1.35 s and 1.0 GB (was
  15.5 s and 5.8 GB), `json.decode` 0.8 s and 0.7 GB,
  `Str.contains_where` 0.46 GB (Last verified: 2026-10-26);
  XO_REFS_SPELL=off checks every module and body. def, doc and
  symbols by name, and query def, api, imports and deps, check no
  function body at all (Checker.decls_only), and query callers only the
  bodies that spell the name: def at the xo root 0.38 s and 0.38 GB, doc
  0.53 s, query callers 0.38 s; in compiler/ 0.24 to 0.37 s (was 1.1 to
  2.3 s and 1.8 to 2.7 GB; Last verified: 2026-10-26). type FILE:L:C,
  query type and query effects still check every body (0.2 s, and 1.1
  to 1.4 s for effects). XO_LOOKUP_EAGER=1 checks every module and body
  for every lookup (TestAgentLookupsEager compares the two). Two modules
  under a root with one module path (the xo checkout's
  `example.com/notes` three times) are checked once: the first, and the
  others named (`"same_path"`, a `note: not checked` line); look one up
  with `--dir` (2026-10-26: their references were missing silently).
  `xo check`, vet and query check each such module on its own. Refs of a std name checks
  only the std and runtime files whose text spells the name: 0.08 to
  0.4 s for a rare name, about 1 s for a common one (`find`, `push`),
  where every std module was checked before (1.2 to 1.8 s on a quiet
  Mac, 5 s under load). `xo refs A B C` answers several names from one
  check (JSON `{"results": [...]}`, in order; exit 1 when one has no
  answer): use it for a sweep instead of one call per name. Last
  verified: 2026-10-26.
  Last verified: 2026-10-21.
- `xo dev edges [--out=FILE] DIR` prints every reference the checker
  resolves in DIR's program, one tab separated line each: the function
  whose body holds it, the declaration it names (path, line, name), and
  the reference itself (path, line, column, name). It is the resolved
  reference graph a name scan cannot give: `s.find_where(f)` resolves to
  `Str.find_where` although the receiver written is a variable, not a
  type. `scripts/stdcoverage -edges=DIR` reads a directory of its .tsv
  files and reports the name scan's coverage against it (2026-10-25: of
  1455 declarations the scan over-claims 129, 86 of them with no
  collision to explain them, and misses none; 43 are interface dispatch,
  which needs implementation edges). A `use go` or `use c` program needs
  its binding first, or the command exits 1 naming it. Last verified:
  2026-10-26.
- `xo edit [--dry-run] rename OLD NEW` renames functions, methods,
  constants, types, fields and enum variants (`Type.member`), and
  interface methods (`Iface.m`, with every implementing type's method),
  every place they are written; a Json or SqlRow field keeps its wire key
  (`new as "old"`); `--dry-run` prints a diff. Last verified: 2026-10-21.
  The edit's second check reuses the first's std parses.
- `xo check --wip DIR` while an edit is half done: unused bindings and
  imports (XO0110) and unused computed values (XO0209) are warnings, so
  only the errors that matter show; check without it before reporting
  done (decision 0293).
- `xo check --no-gc DIR` for code a native build compiles, test code
  included: plain `xo check` accepts what only the native backends reject
  (XO1201, such as a test's own struct converted to `Fs`), which `xo test`
  then fails to build (gotchas/test-fs-wrapper-not-native.md).
- `xo vet` takes a `.xo` file, a std directory (`xo vet std/native/llrt`,
  every runtime module in one check, 0.6 s), or `--std-module=M`.

## Writing low-level arithmetic

- Int word methods (decision 0260, on main since 34dab50f):
  `wrapping_add|sub|mul`, `wrapping_shl`, `shr_logical`,
  `rotate_right[32]`, `swap_bytes`, `mul_hi_unsigned`,
  `lt_unsigned`/`le_unsigned`, lowered to the same instructions as the
  rt.int_wrap intrinsics (spec/std.md). std uses them throughout;
  internal/repotest TestStdUsesWordMethods rejects `rt.int_wrap` and
  `rt.u64_mul_hi` in std outside the methods' bodies.

## Debugging a wrong program or a fault

- A native program that dies on a hardware fault (SIGSEGV, SIGILL, an
  access violation) prints a crash report (decision 0343): the first
  line, the registers, `frames:` and a `traceback:` with files and
  lines. When the traceback is missing (it timed out or faulted) or the
  binary was stripped, `xo dev symbolize PROG < REPORT` names the
  frames from the unstripped PROG (2026-10-22).
- `XO_PASSES=-name,...` turns optional passes off; `XO_PASSES=+name`
  turns on one a target leaves off (none today: hollow is on for
  darwin/amd64 too since 2026-10-26);
  `xo dev bisect-pass
  DIR` finds the first pass that changes a program's output.
- `XO_LOWER=-name,...` turns optional SSA builder lowerings off
  (2026-10-21; compiler/ssa lowerings.xo lists them: `eq_lower`, the
  `a.to_lower() == b.to_lower()` rewrite, and `user_cap_call`, decision
  0307's dispatch on user capabilities, which then work only for the
  runtime's own): bisect inside a commit that changed what the builder
  emits, as XO_PASSES does for the native passes. A wrong name fails and
  lists them; the front end cache keys on it; a new lowering is added
  to lowerings() and gated with `b.lowering_on("name")`
  (TestLoweringsListed fails when they disagree). `xo dev dump ssa
  --fn=NAME` with and without it shows the difference.
- `reduce-diff.py --crash` shrinks a failing program; property tests
  keep the failing program, print the build's output, the culprit pass,
  and the reduced program.
- What the inliner and the register allocator read (2026-10-21):
  `xo dev dump ssa --fn=NAME --stage=build|unwind|landings|cleanup|rc DIR`
  prints only the functions NAME names (`Type.method`, `name`, or with
  the module's name before it: `flate.InStream.push`) after that pass; a
  non generic function main does not reach is declared as a root, so a
  std function works from any program directory. `xo dev dump asm
  --ssa=rc|inline --fn=NAME DIR` prints them after the passes or after
  the native inliner (a function every caller inlined is gone from
  inline; `call sq` becomes its body). `--stage=rc` marks borrowed
  parameters (`borrow v1`). Without --fn the dump is as before.
- `XO_POISON_FRAMES=1` fills frame slots with a poison pattern, so a
  read before a write shows up at once.
- `XO_DCE=off|debug|list` controls and lists native dead function
  removal.
- `XO_TRACEBACK=locals` (decision 0268): a native fault report lists each
  frame's variables under it (Int, Float, Bool, Str text, a list's first
  elements, struct fields, enum variants), read from the program's own
  DWARF; `<unavailable>` means the frame no longer keeps the value.
  Off by default; toolchain.md has the details.

## Testing

- New intrinsics (2026-10-25): an rt intrinsic that lands before its
  first std caller (a seed has to know it first, gotchas/seed-older-than-intrinsics.md)
  still lands with a test that builds a caller on every backend that
  lowers it, such as a std-internal `_test.xo` run with `xo test
  --std-module=MODULE --backend=arm64` and `--backend=llvm`. rt.case_fold16
  (115e372a5) landed without one, and its LLVM lowering emitted invalid IR
  twice (an i64 where a ptr was due, then hexadecimal integer literals),
  found only when the caller arrived (eec0768d3).

- Docker (2026-10-25): start Docker Desktop for a run with
  `~/xo-lead/agents/docker-up.sh` and stop it after with `docker-up.sh stop`,
  never `open -a Docker`: twice Docker Desktop came up "running" while
  `docker info` hung, and the waiting agent lost 15 minutes. The script
  waits for the daemon, force stops and starts it once when it does not
  answer in 90 s, and exits 1 for the lead after a second failure.

- Default changes (2026-10-25): a push that changes a default (a flag on
  by default, a default-on pass) runs its full suites on the exact tree it
  pushes, after its last commit, not on an earlier commit of the same
  work. `--trimpath` (303dc270d) passed a full internal/arm64 run made
  before its last commits, and TestBackendCacheObjectsAcrossDirs, which
  expects absolute DWARF directories, was red on main for about two hours
  (fixed in ec8602220). An inliner or cost-model change also runs the full
  native-vs-LLVM instruction sweep before its quiet --ab, so the window's
  rows are the sweep's movers rather than the author's choice.

- Runtime changes (2026-10-23): a change to std/native/llrt pool, tasks,
  mutex, sched (the scheduler and its shared words) runs internal/llvm's
  ThreadSanitizer tests in its gate, `go test -short -count=1 -run
  'TestRuntimeThreads|TestRuntimeEventLoop|TestRuntimeAlloc|TestRuntimeRealtime'
  ./internal/llvm`, more than once (a race hit 3 runs in 4): the runtime is
  instrumented in a --race build, so a plain store to a word every worker
  writes is a reported data race, and the conc rows built with --race ran too
  briefly to see it (935c3779e, fixed in b33e9ed22). Share such words with
  relaxed atomics (rt.slot_load, rt.slot_store).

- Hung xo in a test (2026-10-23): xotool.Build and asm run xo through
  `xotool.RunWatched`; a child still running after `XO_BUILD_LIMIT` (a Go
  duration, default 20m, `0` for none) gets `xo dump PID`, `lsof -p PID` and
  (macOS) a 3 second `sample` appended to its output, then SIGKILL, and the
  build counts as failed. The report names the waiting task, which a bare
  stack sample of an idle runtime cannot. Last verified: 2026-10-23.

- Changing a printed text (an error message, a diagnostic, a failure
  report, a CLI line): before pushing, grep every expected-output file
  for the old text, `git grep -l -F 'OLD TEXT' -- '*want*.txt'
  '*_test.go' '*_test.xo'`, and run or update each test that names it
  (lead, 2026-10-23: three main failures that day were stale want files
  after a text change: test_go_callback_deadlock, net_only,
  TestBuildSettings). A text built at run time (a derived Display,
  `${...}`) is not in any source literal, so grep for the printed form.
- `xo test --std-module=MODULE` runs a std module's `_test.xo` files with
  private access (decision 0262); internal/stdtest runs every std module
  that has tests. Tests under std/ and compiler/ live only in `_test.xo`
  files. Only std/native and std/native/llrt have them (2026-10-24; `find
  std -name '*_test.xo'`): another module (std/time, std/strings) runs no
  tests, and its checks are internal/stdtest's corpora (TestStdCorpora)
  and the goldens. A run of no tests says "xo test: no tests in ..." on
  standard error and still exits 0; gate scripts pass `--fail-empty` so
  it fails (2026-10-24: a gate's `--std-module=time` printed "ok: 0
  passed" and read as a pass).
- internal/llvm timeout (2026-10-24): the package's goldens need 46+ minutes
  (each builds and links with clang), and go test's default timeout is 10
  minutes, so a bare `go test ./internal/llvm` FAILs at 600s naming no test,
  after ten minutes of work (two agents lost a run each that evening). Run
  one golden with `-run 'TestGolden/NAME$'` (NAME is a directory under
  internal/llvm/testdata/golden; XO_TEST_GO_PARITY=1 adds the Go backend
  comparison), or the whole package with `-timeout 90m`. The test binary
  itself now stops such a run at once with that advice
  (internal/llvm/golden_timeout_test.go), and -short is unaffected.
- One golden on every golden driver (2026-10-26): `-run 'TEST/NAME$'`
  builds only NAME. internal/llvm TestGolden and TestGoldenGoParity,
  internal/arm64 TestGoldenRun and internal/gen TestGolden build inside
  each subtest; TestSelfhostWasm (XO_SELFHOST_TIER=merge) and the
  internal/devgoldens tests (TestDevGoldensArm64, LLVM, Go, ELF) read the
  pattern (internal/runfilter) and compile only the goldens it selects,
  then report one subtest a golden. Before that change a one-golden
  TestSelfhostWasm compiled every golden (about 14 minutes) and the
  devgoldens tests ran every program. A pattern that names a test but no
  subtest still runs them all.
- A runtime fault cannot run inside a test binary, so its corpus is one
  small program per case: internal/stdtest/testdata/faults/AREA/NAME/
  main.xo with the behavior recorded in its want.txt (exit status, stdout,
  and the `fault: ` stderr line). TestSliceFaults builds and runs every case
  on the corpus backends (go, arm64, llvm) outside -short, about 40 s.
  `go run ./scripts/stdcorpus` writes the programs (scripts/stdcorpus/
  faults.go) and `go test ./internal/stdtest -run TestSliceFaults
  -fault-update` records want.txt from the Go backend, whose message and
  position are the oracle: a native backend that says anything else fails
  the test. The Str and Bytes slice cases are spec/core.md 3.6.
- The request head parser has its own differential (2026-10-26):
  `go test ./internal/stdtest -run TestHttpHeadFuzz` generates seeded raw
  header blocks (obs-fold lines, tabs, control bytes, repeated names, LF
  endings, an empty line mid-block) and compares `http1.parse_head` with
  `net/http.ReadRequest`, one `xo test` case per draw, so a failure names
  the case. The parser is a private runtime module, so no corpus can reach
  it: the test copies std into a temp dir and writes a generated
  `http1_test.xo` there (the `stdprobe_test.go` shape). `XO_HEADFUZZ_N`
  sets the case count (default 300) and `XO_HEADFUZZ_BACKENDS` the
  backends (default arm64); 2000 cases run both arm64 and LLVM in about
  10 s. The oracle applies the server's own name rule
  (`ValidHeaderFieldName`) and keeps `Host` valid: `ReadRequest` is not the
  server layer (gotchas/readrequest-not-the-server-layer.md). The bytes are
  the Str the parser sees, so invalid UTF-8 arrives as U+FFFD (decision
  0259); the byte-to-text step has its own tests.
- The response head parser has the same shape (2026-10-26):
  `go test ./internal/stdtest -run TestHttpResponseFuzz` generates seeded
  raw response heads (bad status codes and versions, Content-Length and
  Transfer-Encoding shapes, Trailer, Connection, Pragma, obs-folds,
  control bytes, LF endings) and compares `http1.parse_response` with
  `net/http.ReadResponse`; `XO_RESPFUZZ_N` and `XO_RESPFUZZ_BACKENDS`
  work as for the request fuzz, 2000 cases on arm64 and LLVM take about
  11 s. Two `RespHead` fields are deliberately not Go's public ones
  (`clen` is readTransfer's realLength, `close` is shouldClose): the
  oracle derives them from the head, see the gotcha. The field order and
  a refused head's error text are not compared yet.
- `go run ./scripts/stdsync [-check]` after changing std/prelude.xo: one
  command for every file a prelude change derives, spec/std.md's Appendix A
  index (every prelude function and method by type, internal/stdindex), the
  habit goldens (their want.json text and fixed.xo name prelude methods;
  regenerated only when the prelude's declarations changed, checked against
  cmd/xo/testdata/habits/prelude.sha256), and std/rt's generated files.
  `-check` names each stale artifact and prints the one command that fixes
  them all. TestStdIndexCurrent and stdgen -check name it in their failure
  messages too.
- `go run ./scripts/stdcoverage` prints every std function and method no
  corpus or golden calls (internal/stdcoverage): std/*.xo and
  std/native/prelude.xo against the calls in internal/stdtest's corpora,
  internal/llvm's and internal/arm64's goldens, internal/gen's goldens,
  cmd/xo's testdata, bench/perf's benchmark programs and scripts/,
  following std's own calls and the prelude lowering
  (decision 0074). The -short TestStdAPICovered fails for an uncovered
  declaration that internal/stdcoverage/allow.txt does not name with its
  reason, and for an allowlist line that matches nothing; a new std
  function or method needs a corpus, a golden, or such a line.
  The name scan cannot tell two receivers apart (a corpus writes
  `xs.map(f)`, so the receiver of a method call is a variable), so
  `xo dev edges` resolves the calls instead: over the corpus and the
  goldens, 214 declarations are uncovered by resolution against 128 by the
  name scan, and 86 declarations the scan calls covered are reached by no
  edge (2026-10-25, internal/stdcoverage UncoveredResolved;
  `-edges=DIR -dirs=FILE` compares the two over the same programs). Those
  numbers predate the impl-file sites (2026-10-26, next bullet), so most of
  the 86 were sites the edges never held; the comparison has to be re-run
  with them. A like-with-like comparison needs the same programs on both
  sides: `go run ./scripts/stdcoverage -programs` prints the 890
  directories the sweep runs `xo dev edges` over (the corpora, the
  goldens, bench/perf, scripts), which is the file `-dirs` reads.
  On the full sweep (2026-10-26, all 890 program dirs, impl-file sites
  included, 219 million edges, plus a re-sweep of the three programs
  commit e8fe6f28f added or changed): the name scan calls 126 declarations
  uncovered and the resolved edges call 187; 61 declarations the scan
  credits are reached by no edge and none goes the other way, so the scan
  is the safe direction and remains the gate. The two causes of the 61,
  which is what the audit is for: interface dispatch (a call through an
  interface resolves to the interface's method, never to the
  implementation, so `<Type>Err.display` looks unused) and bare-name
  collisions, the scan's known limit and not a corpus gap: `prelude.Buf.push`
  has no reference while `prelude.List.push` has 7,553,
  `prelude.Bytes.contains` none while `prelude.Str.contains` has 525,
  `prelude.Duration.abs` none while `prelude.Int.abs` has 4 (checked with
  `xo refs`). A line the edges contradict is worth a look; a declaration
  the edges cannot reach is not by itself proof of a missing corpus.
  The 61 split as 41 marked `[impl]` (30 of the 31 `.display` methods plus
  eleven others) against 20 not: `sql.Driver.connect` and eight
  of `sql.DriverConn`'s methods (an interface only a program's own driver
  implements, the one corpus gap left), `prelude.read_or`,
  `read_text_or`, `texts`, `llrt_concat_int`, `Stream.close`,
  `internal.SharedLock.unlock`, `metrics.latency_buckets`,
  `go.Error.message` and `go_type`, `http.StatusError.status` and
  `display`, the cold paths no program in the sweep drives. Do not count
  a declaration the sweep's base predates: the sweep's xo is older than
  the corpora landed during it, and re-sweeping only the programs a later
  commit added or changed is the cheap fix (three runs and one `-edges`
  run found six declarations the base had left uncovered, 2026-10-26).
  The audit's procedure, and why a sweep is not a gate: a sweep is 3.5
  hours of `xo dev edges` and 22 GB of text (1.5 GB gzipped), against
  seconds for the name scan. Run it as shards of about 90 programs (each
  `HEAVY_CLASS=gate`, about 2 minutes: decompress the shard's parts, one
  `-edges` run, delete them), never one long job (owner, 2026-10-26), and
  merge the shards' lists. Because the roots are the non-std site paths and
  the walk only crosses edges out of a root and then through std bodies, a
  declaration is reached by the sweep exactly when one program's own edges
  reach it, so the reached sets union and the uncovered sets intersect.
  `-list=DIR` writes `uncovered.txt`, `over.txt` and `under.txt` with the
  `[impl]` mark, which is what makes a sharded merge faithful. How to merge,
  and which list to merge how: `uncovered.txt` intersects across the shards
  (a declaration is uncovered by the sweep only when every shard leaves it
  uncovered); `over.txt` must NOT be intersected shard by shard, because
  each shard was run with an empty dirs file (`scanned` is everything, so
  over is empty) and a project run instead can count a declaration as over
  whose only reaching edge sits in another shard. Derive over after the
  merge: the merged uncovered set minus the name scan's own uncovered set,
  read from one `-dirs` run over all the program dirs (the same file the
  sweep used). The mark and the two lists are the audit's product. Do not
  record the audit's entries in allow.txt, which means "uncovered and
  accepted": a declaration the edges contradict is documented with its
  reason in the audit's results, not accepted.
  Since 2026-10-26 the walk also crosses from a std module's front
  declaration into its implementation: `implAliases` reads std/native/<module>.xo
  for positions only (they are not reported declarations, which are the API
  surface) and links same module, symbol and kind, so a covered front
  declaration follows its body's edges and covers its twin. A declaration
  that exists only in the body file (std/native/sql.xo's Pool) is a node of
  the walk too (`loadImplDecls`, which reads std/native/llrt/*.xo as well),
  unlisted and uncounted, so body -> native helper -> declaration and body ->
  llrt helper -> declaration chains are followed rather than dropped. An llrt
  file's module is its base name and can equal a front file's
  (std/native/llrt/trace.xo against std/trace.xo), so its declarations never
  join an alias pair. Before that, every declaration called only from an
  implementation body read as unreached; measured on four programs, uncovered
  went 1357 to 1257 and 100 declarations were newly reached, among them
  `sql.Driver.connect`,
  `sql.DriverConn.begin/commit/close/exec/is_broken/ping/query/rollback`,
  `prelude.read_or`, `read_text_or`, `prelude.Str.trim`, and 8 that llrt
  helpers reach (`Int.wrapping_shl`, `Int.mul_hi_unsigned`,
  `Int.shr_logical`, `List.insert` and `prelude.fault`, with their lowered
  helpers; std/native/llrt/alloc.xo and asn1.xo are the callers). Keep each
  shard's .tsv until the audit's numbers are superseded: the first sweep
  deleted them after each shard, so its figures can only be recomputed with a
  second full sweep.
- `xo dev edges [--out=FILE] [--impls] DIR` writes the checked program's
  resolved references, one tab separated line each:
  SRC_PATH SRC_LINE SRC_NAME TGT_PATH TGT_LINE TGT_NAME SITE_PATH
  SITE_LINE SITE_COL SITE_NAME, where SRC is the function whose body holds
  the site (line 0 outside one) and TGT is the declaration it resolves to
  (compiler/cli/servers/edges.xo). `--impls` adds an edge from each
  interface method to the method of every type whose method names cover
  the interface's (the checker's structural rule, the one
  rename_iface_method uses), with the interface method as SRC and 0 as the
  site column; a call through an interface names the interface method, so
  without these edges every implementation looks unused. TestDevEdges runs
  it on the strwhere golden.
  The check preloads `native/NAME` for every `std/NAME.xo` that has a
  `std/native/NAME.xo`, so the implementation files' bodies are checked
  and their sites are in the output: without that the eight impl files
  that nothing imports (the prelude's, big, http, internal, log, sql,
  testing, time) had no site and no target at all, and every declaration
  they implement looked unreachable (2026-10-26). The cost is a full
  runtime check a program: 16 s for a small corpus against 0.3 s for
  `xo check`, so a sweep over 800 programs is hours; run it in a bulk
  slot, and sample when a full sweep is not needed.
  A `use go` or `use c` program needs its binding before it checks: with
  the binding missing the check stops before names resolve, and the
  command now exits 1 with that reason. It wrote only the standard
  library's references and exited 0 until 2026-10-26, so a per-directory
  sweep counted every `use go` golden as a program with no references of
  its own (the first coverage sweep lost them all, see its results file).
  Run `xo bind --dir=DIR <go imports...>` in the module first, or use a
  golden that commits its `.xo/gobind`; the binding can be deleted after
  the sweep. TestDevEdgesMissingBinding holds the exit 1.
- `go run ./scripts/stdfuzz` is the backend differential fuzzer: it
  generates one Xo program of seeded random inputs through pure std
  functions (strings, bytes, strconv, json encode/parse, runes, path,
  url, base64/base32/hex, time formatting and parsing, regexp, csv,
  netip, big, compress, archive, mime, multipart, xml, sql, http,
  template), including the Str and Bytes methods that take a Rune
  closure, and the
  internal.crypto_op test paths (cryptostream, x509 on mutated real
  certificates, ECDSA P-256/P-384 verify and ECDH, RSA verify and sign),
  plus a limits area at the chunk and buffer edges (sizes up to 1 MiB
  through counts, search, replace, split, the codecs, JSON, CSV and URL
  escaping), a runes area that pushes every Rune predicate, case
  mapping, utf8_len and both quotes over the class, block, bitmap,
  surrogate and scalar-range edges, and a maps area of put, get, remove,
  contains, len, update, put after remove and re-put over six key and value
  type pairs (Str and Int keys, Int, Str, Option, List and struct values),
  with growth cases around powers of two to 100,000 entries; the phase
  summaries print the length, an order sensitive mix and the first and last
  entries in iteration order, because a Map is insertion ordered (core.md
  2.4, decision 0091) and that order is the contract a rewrite can break
  (the Go backend is the oracle); it builds and runs
  arm64, LLVM and the Go backend at once (at most three at a time; since
  2026-10-25, when they ran one after another and a seed of every area took
  about nine minutes), and compares the outputs line by line. A divergence
  prints the case, its source and a `-only=NAME` rerun; report it, do not
  fix it in the tool. `-seed` and `-n` widen a run, `-area` narrows it, and
  `-keep=/DIR` keeps the generated program. `-release` builds the arm64 and
  LLVM programs with `xo build --release` and runs the binaries, so the
  release driver's contents are compared too (the DWARF readers left out,
  and the hollow pass that empties unreached functions); the Go backend
  stays an oracle run, and a plain run never exercises those paths
  (2026-10-24).
- The private-module probes compare what only a std module can reach,
  against Go, on a seeded or hand written table (2026-10-26):
  TestHttpHeadFuzz and TestHttpResponseFuzz (internal/stdtest) generate
  seeded raw request or response heads, write one xo test per case over
  `native/llrt/http1`'s parse_head or parse_response into a temp std copy,
  and compare with net/http's own parse. `XO_HEADFUZZ_N`/`XO_RESPFUZZ_N`
  and `XO_HEADFUZZ_BACKENDS`/`XO_RESPFUZZ_BACKENDS` size and target a run
  (defaults 300, arm64; the corpora's backends are arm64, llvm and go, but
  llrt is native only, so the Go backend is not one of these). The harness
  parses `xo test`'s "N passed, M failed" line and fails unless it equals
  the generated case count, so a file that is not picked up cannot pass a
  comparison of nothing; the count is logged per backend. TestHttpSmuggle
  is the smuggling table: 41 hand rows (Content-Length duplicates and
  conflicts, CL with Transfer-Encoding, short bodies and a CL that
  swallows pipelined bytes, TE lists, folds, casing, bad chunk framing,
  Host shapes, name bytes, and header values with valid UTF-8 and with
  obs-text bytes) are sent to a Go net/http server on
  127.0.0.1, which answers each request it accepts with the framing its
  handler saw (Content-Length or chunked, the length, and the body bytes
  read, or its error and the bytes before it); the Xo side reads the same
  bytes as a Str the way the server does (rt.cast[Bytes, Str], no
  conversion), takes the framing from its own Head (clen, chunked), then
  chunked for the body, slices bodies from the raw Bytes, and reports short
  data as an error with the bytes
  read so far, never as a body taken from what follows. The comparison is
  that framing line
  per request, joined in order, so a body framed differently by the two
  sides fails even when both accept it, and the pipelined row pins where
  the next request starts; the table fails if Go accepts or rejects fewer
  than five rows or fewer than five carry framing, so it cannot go
  vacuous. The port's two documented non-Go field shapes (RespHead.clen
  is readTransfer's realLength, close is shouldClose) and the folded
  Connection trap are in gotchas/readrequest-not-the-server-layer.md. A
  divergence is reported to the lead before std changes, never fixed in
  the probe. TestHttpUrlFuzz is the same shape over `native/llrt/neturl`:
  2000 seeded URLs (a structured shape, an absolute shape with an
  authority, and random bytes; a sixth cut short or byte changed) against
  net/url's Parse and ParseRequestURI, `XO_URLFUZZ_N`/`XO_URLFUZZ_BACKENDS`
  sized (defaults 2000, arm64 and llvm). Three rules make it faithful: a
  Parse input is cut at the first '#' (every caller cuts the fragment, and
  Go's Parse would otherwise validate escapes in a fragment nobody reads),
  ParseRequestURI gets the target as it is (a request target has no
  fragment) and an empty target is "empty url" (http1's own error), and the
  whole answer line is compared as hex, because the fields can hold bytes
  that are not UTF-8; each component is rendered the way neturl keeps it,
  an invalid byte staying its %XX escape (decision 0259). It exercises
  scheme, userinfo (validated only: neturl returns no user), host with
  IPv4, IPv6 with a zone and a port, a port that is not a number, escaped
  and empty hosts, multi-byte and invalid bytes, path escapes, and query
  text, and it fails if the answers stop covering errors, schemes, hosts,
  paths and queries. scripts/gate.sh has two steps for them (lead,
  2026-10-26): `fuzz` runs these four probes, non -short, at their
  defaults, 20 s alone and 42 s while corpora-all ran, both measured
  2026-10-26, and every tier runs it; `corpora-all`
  runs every corpus on every backend (TestStdCorpora with no /go/ suffix),
  10 min 32 s measured 2026-10-26 mostly in the LLVM corpora's clang
  links, and the 8-hourly full tier runs it every time. Both are where
  0.33.0's exit criterion 6 is met.
  A divergence is reported to the lead before std changes, never fixed in
  the probe.
- `go run ./scripts/stdgen` after changing a std type (a struct, enum,
  newtype, or interface in std/*.xo) or a std function's signature:
  regenerates std/rt/std_gen_M.go, the Go backend's declarations of
  module M's std types, and std_sigs_M.go, which makes `go vet ./std/rt`
  name a hand written function whose signature drifted (toolchain.md),
  one pair per module since 2026-10-21, so two pushes that change
  different modules do not conflict there; the -short `TestStdGenCurrent`
  fails until it is run. Never edit those files by hand, and resolve a
  conflict in one by regenerating.
- `scripts/gate.sh WORKTREE STEP...`: an agent's gate as named steps (its llvm step runs only the LLVM goldens the change touches, toolchain.md "A gate's goldens").
- `scripts/bisect.sh [--pkg=PKG] [--timeout=D] [--keep] [--skip-verify]
  TEST GOOD BAD`:
  the first commit between GOOD (passes) and BAD (fails) where TEST
  fails, and its author: TEST a go test -run pattern (the package found
  at BAD) or a shell command (it has a space) run with that step's
  XO_CLI. Each step's xo is built with this checkout's seed choice
  (era-correct seeds), every xo cache private and deleted after. It
  tests GOOD and BAD first and stops (exit 3) when GOOD fails or BAD
  passes (a GOOD that already failed once named the commit after GOOD,
  2026-10-22); `--skip-verify` skips that check. After editing it, run
  `scripts/bisect_test.sh` (seconds, no xo build). Run it
  as `HEAVY_CLASS=bulk ~/xo-lead/agents/heavy.sh NAME scripts/bisect.sh
  ...`, once the lead has named you the bisect owner (CLAUDE.md).
  A crash or a failure that could be intermittent (a fault, a signal,
  a race, a timeout) is first rerun on BAD about 10 times to measure
  how often it fails, and bisected only if it fails every time; an
  intermittent one goes to a repro loop and its owner instead (lead,
  2026-10-27: an xo SIGSEGV that hits about 1 in 32 builds was bisected
  twice, and its GOOD failed, before the repro showed it predates the
  range).
- `scripts/progen-batches.sh`, the amd64 progen runner, TestArithOracle.
- `scripts/codesize.py BINARY`: where a darwin/arm64 build's `__text` goes (functions, modules, generic copies, instruction kinds, whole value word copies, release runs, constant addresses); bench/results/xo-code-size-2026-10-18.md.
- `scripts/docker-newest-go.sh DARWIN_XO ARCH WORK` (owner, 2026-10-27;
  decision 0393): `use go` against the newest Go release, read from
  go.dev/VERSION when it runs and printed first, in its official golang
  image, with an xo laid out as a release (a Linux xo cross built with the
  Go backend, std, xo-gobind built with this Mac's Go, the helper's source
  under lib/xo): internal/gen's go_* goldens and a bind of strings, strconv
  and encoding/json, PASS or FAIL each, exit 1 on any failure.
  `XO_ROOT=DIR` tests another checkout (main, to see a bug before its
  fix). Run it as `~/xo-lead/agents/docker-up.sh && HEAVY_CLASS=bulk
  ~/xo-lead/agents/heavy.sh NAME scripts/docker-newest-go.sh bin/xo arm64
  WORK; ~/xo-lead/agents/docker-up.sh stop` (it needs the network for the
  version and the image). Last verified: 2026-10-27.
- `scripts/usego-workflows.sh XO OUTDIR` (Lang 2, 2026-10-30): the Go
  workflows suite, bench/usego-workflows (chi, zap, aws S3, pgx, toml,
  yaml written in Xo against the real modules, built with --backend=go
  and run); PASS, FAIL, or XFAIL/XPASS for a workflow
  bench/usego-workflows/EXPECTED lists with the work that fixes it; exit
  1 on an unexpected failure. It needs the network (the Go proxy) and
  leaves each workflow's binding, build and run logs under OUTDIR. Run it
  as `HEAVY_CLASS=bulk ~/xo-lead/agents/heavy.sh NAME
  scripts/usego-workflows.sh bin/xo OUT` after a `use go` change; results
  in bench/results/usego-workflows-*.md. Last verified: 2026-10-30.
- `scripts/arm64-identity.sh`, `scripts/docker-goldens.sh`,
  `scripts/intel-goldens.sh XO HOST REMOTE_DIR WORK` (the Intel laptop;
  owner rules in the lead's notes: one new directory, nothing deleted),
  `scripts/macho-link-compare.sh` (`scripts/macho-symview.py code|data
  EXE` prints an executable's code or data by symbol, for its
  differences).
- `scripts/gate-http.sh OUTDIR [STEP...]`, `scripts/fuzz-http.sh OUTDIR
  [SECONDS]`.
- `xo test --watch DIR` reruns only the tests an edit reaches (a test
  that names a changed function, an edited or new test; `a` and Enter
  runs all); `xo run --watch DIR` restarts the program only when the
  built binary changed (SIGTERM, `--grace`, then SIGKILL). `--json` gives
  a line a cycle and `last.json` under `<user cache>/xo/watch/HASH`, with
  the builds (decision 0267; nothing is written into the project since
  2026-10-22).
- Test programs started by Go tests run in their own process group and
  are stopped (TERM, then KILL) when the test ends (testtmp.Guard).
- A long package suite that fails only some tests: when the fix changes
  only those tests' own code, rerun only them (`go test ./PKG -run
  'TestA$|TestB$'`, a gate), not the whole suite; rerun the suite only
  when the fix changes code other tests use (2026-10-24: internal/arm64's
  37 minute suite reran in full for a 1.6 s TestMachoBinCache fix and held
  a bulk slot and the measurement window).
- A change to an xo command's flags, `-h` help or printed output runs
  the full `go test ./cmd/xo` suite (about 4 minutes), after `go test
  ./cmd/xo -run '^TestCLIHelp$' -update-help` and a look at the diff; and
  a `-run` pattern that matches nothing passes as "ok" (with -v, "no tests
  to run"), so check the count of tests it ran (2026-10-24: a gate's
  `-run 'TestHelp'` matched nothing, TestCLIHelp/test failed on main).

## Measuring

- Where an `--ab` runs (owner, 2026-10-30): a claim that is not darwin
  specific runs on the linux/arm64 offload host with
  `~/xo-lead/agents/ab-host.sh BASE NEW NAME [--rounds=N] -- self|edit
  PROG|PROG` (offload.md; `edit PROG` is the warm rebuild of a program,
  the claim of a front end, cache or linker change); it needs no heavy.sh
  slot and no Mac measure window. `self` has both xos build NEW's
  compiler/xo; when NEW adds a std function the compiler calls, BASE
  cannot build it ("round 0: A: exit 1"), so use `self=a` (BASE's
  sources, which NEW's std also builds; 2026-10-30).
  The Mac's measure windows are only for darwin only claims (Mach-O,
  the macOS scheduler or syscalls) and Mac memory footprints.
- A placement claim (a sweep over XO_ARM64_PAD, an alignment, a layout)
  carries the one-binary control beside it: the same statistic with one
  binary in every slot (16 slots x the rounds); only a spread above that
  floor is placement. A run under about 100 ms varies about 14 percent
  run to run on the Mac even in a quiet window (interp, 50 ms: one
  binary as 16 placements gave 1.139, the same as every sweep that day,
  2026-10-29, gotchas/placement-sweep-noise-floor.md); a 600 ms run's
  floor is under a percent (collatz 1.008). Make the run long or carry
  the floor (owner, lead, 2026-10-29).
- `xo bench --ab` with no --max-load counts a round as loaded when a side
  starts at the load sampled before the first round plus 3 (at least 6);
  the load line prints that baseline (2026-10-28,
  gotchas/bench-fixed-load-limit.md).

- `xo vet --cost --cost-std DIR` notes the cost patterns (XO0907 to
  XO0914) in the std code program DIR reaches; cmd/xo TestVetCostStd runs
  it over bench/perf and keeps std free of new XO0913 and XO0914 notes
  (allow-list: cmd/xo/testdata/vetcost-std-allow.txt, a reason a line).
- A new std module, or a port of a Go package, reads
  knowledgebase/std-performance-guide.md first and ships a Go twin
  (bench/perf/<module>, twincheck) with a timing row in the std timing
  audit (owner, 2026-10-28).

- Measure the build that ships, and check a stand-in before trusting it
  (lead, 2026-10-27; Last verified: 2026-10-27): three 0377 runs measured
  a fixed cap with no raise while the default to flip was mode 3, and
  were dropped after waiting; a faster stand-in (the HTTP server's cold
  build for the self build) is calibrated first against a case whose
  answer is known (fixed cap 4: +26% on the self build), and a mistyped
  flag (`--env-b X` for `--env-b=X`) made three jobs exit in a second, so
  check one job's log before queueing its siblings.
- One A/B per measure job, about 30 minutes at most (lead, 2026-10-27;
  Last verified: 2026-10-27): every slot run waits while a measurement
  holds the measure lock, so a job of four 20 round self build A/Bs
  (la10, 02:07 to about 04:15) held both slots idle for two hours while
  two milestone gates waited 115 minutes. Queue each A/B as its own
  `HEAVY_CLASS=measure` job, launched with run_in_background (never a
  trailing `&`: the job's PID is then lost), so gates run between them.
- `xo bench --ab` only as its own measure job (2026-10-27): inside a gate
  or slot run it would wait for the quiet window that waits for that gate,
  so it exits 2 there with that advice (XO_HEAVY_SLOT set by heavy.sh, not
  `measure`, not the window's own run, and no --no-guard or --guard;
  gotchas/bench-ab-inside-gate-deadlock.md). A quick instruction count in a
  gate reads `/usr/bin/time -l`'s "instructions retired" line instead.
- Before queueing a bulk gate, run its new tests alone in the gate class
  (lead, 2026-10-27): g499's first bulk run failed only on its new
  test's own expectations (a line number, a type annotation), and the
  rerun queued for another hour.
- Ceilings before building (lead, 2026-10-27; Last verified: 2026-10-27):
  before building a speedup step, measure the most it can save. Stub or
  skip the phase on a throwaway branch (never pushed; output ignored),
  time the target quiet with n=5 interleaved in the gate class, and only
  build steps whose ceilings add up to the goal. A budget summed from
  per-phase CPU time is not a wall estimate: ADR 0382 stage 2 was built
  and gated against a CPU budget and broke even, and stage 3 plus the
  chain delivered 11 ms of about 35 planned, while the oracle table
  (Lang 1, o497) priced every remaining candidate in about an hour,
  dropping two of them at about 0 ms. A stub that changes a pass's output
  measures its downstream effects too, so check a candidate's own share
  with a same-output variant or its profile before trusting the ceiling
  (2026-10-27: the prune_unwind stub's -17.5 ms on HTTP was downstream;
  the pass made 2 ms cheaper moved the wall 0 ms).
- Fewer instructions is not faster (agent poll 2026-10-26: the most
  misleading trap; Lang 3's cut_suffix ran +1.6% instructions and +9.7%
  wall, an IPC drop). `xo bench --ab` on macOS reports cycles,
  cycles_per_op and ipc beside instructions, each with its B/A interval:
  explain a wall change that instructions do not with the cycles and ipc
  rows, and quote them with the instruction ratio. Linux GNU time has
  neither counter (`n/a`).
- Code size is measured with `go run ./scripts/sizebreak [-top=N] [-json]
  [-reach] BINARY` (Mach-O and ELF; use it through heavy.sh like any build
  or test): it sizes every text symbol by the gap to the next one, checks
  the gaps against the `__text` size and fails when they do not cover it,
  groups the symbols by module from the `_xo_f_<module>_2e<name>` mangling
  (an ELF binary's symbols have no leading underscore: `xo_f_...`, `xo_h*`;
  the Map families xo.map, xo.mlog and xo.maph broken out per element
  type, then the hidden `_xo_h*` helpers and the non-Xo symbols), and
  prints the groups and the top symbols. `-reach` adds a lower bound on
  unreachable code (direct branches plus materialized addresses, arm64
  Mach-O and ELF, whose roots are `main`/`_start` without the underscore;
  indirect calls are not followed, so a listed symbol may still run). A
  size claim quotes sizebreak's `__text` and group table, not hand-run
  `nm`/`otool` numbers, so two reports compare the same way (lead,
  2026-10-24).
- Before starting on a ranked gap (lead, 2026-10-22: five items that
  day came from old tables: two already fixed, one misread, two
  stale), re-measure it on today's main and cite the figure's file:line
  and date; rankings name their source the same way.
- An A/B pair differs in the change only (lead, 2026-10-24: three runs
  thrown away that day): build both programs from one revision, the base
  from that revision's file and not a moved origin/main (`git show
  origin/main:FILE` picked up another push), with xos built from committed
  trees (a dirty tree's xo, XO_DIRTY set, runs on cold caches and measured
  5 times slower), and print each xo's `xo version -m` and each build's exit
  status (`set -o pipefail`; a failed xobuild behind `| tail` left the old
  xo in place). Start a measurement with `HEAVY_CLASS=measure
  ~/xo-lead/agents/heavy.sh NAME CMD...` (2026-10-26): it waits its turn
  (one measurement at a time), waits out the lead's full tier or main
  check without a window, opens its own quiet window, lets the running
  slots and gates finish (at most 30 minutes; past that it exits 75,
  refused, rather than measure under load: start it again), runs, and
  closes the window, so no lead go is needed; never start one by hand when the
  load drops (two runs started that way overlapped another agent's
  window).
- Build both sides of an A/B (`xo build` in a scratch script, `xo bench
  --ab`) with `XO_INLINE_STRICT=1`: without it a module that fails
  verification builds without inlining and says so in one warning line
  that `| tail -1` or `> /dev/null` hides (gotchas/arm64-inline-verify-fallback.md;
  observed again 2026-10-24 on an RC change). gate.sh, hourly.sh, the Go
  tests and bootstrap-xo.sh already set it. Since 2026-10-24 such a
  binary also says so in `xo version -m` (`build inline=fallback`), and
  `xo bench --ab` refuses it (exit 2) unless `--allow-fallback`.
- Before asking for a Windows CI run (lead, 2026-10-22: four of five
  crash and PDB runs failed on setup, not on the code): build the
  script's programs on the Mac with the oldest seed that CI may use
  (an import or intrinsic the seed lacks fails xobuild's smoke), run the
  script's steps under Wine, and check every tool it calls exists on
  the runner (no clang, no cdb by default).
- Before writing a bench results file, run `go run ./scripts/twincheck DIR`
  (through heavy.sh): it builds the bench program and its Go twin, runs
  both, and compares the lines they print, so a results table's checksums
  are known to agree. It found that bench/perf/aesgcm's and chacha's `open`
  checksums had never matched (Xo's internal.crypto_op("aes.open" and
  "chacha.open") returns a status byte then the plaintext, which the twins
  did not count; 2026-10-24). A bench directory's `twin.txt` says how to run
  it: cases (`XOPROG GOPROG [ARGS] [-- GOARGS]`, `-` for the directory's
  default program), `compare fields N` for a program that prints its own
  timing as another field, `env KEY=VALUE` for a smaller iteration count,
  `require MODULE VERSION` for a twin importing outside the standard library
  (built offline from the module cache), and `skip REASON` for a server.
  It is a pre-results step, not part of the hourly tier: it builds every
  program twice. A mismatch in a bench that has a results file is a finding
  to report, not to fix quietly.
- A change to the build cache's lookup, store, listing or prune paths
  is timed against the real shared user cache (tens of GB, every shard
  full), not only a small private one, before it is pushed (lead,
  2026-10-24: 284b87da9 listed whole shard directories with a stat per
  entry; fast on a private cache, 120 s timeouts on the 19 GB user
  cache, main slowed for 1 h 40 min until 712928e22 reverted it; gotcha
  fs-read-dir-stats-every-entry.md): one cross-target `xo dev goldens
  --build-only` of a few goldens with the new xo, against the user
  cache, with its wall time beside main's.
- A slow build or run starts with a breakdown before any hypothesis
  (lead, 2026-10-23: four Windows CI runs and about five agent-hours
  went to cache theories, cold pack, prune, cold keys, pack size, until
  `--phase-times` showed 538 of 564 s in link; the cause was xo.exe
  built with the Go backend): the first run of an investigation sets
  `--phase-times` and `XO_CACHE_STATS=1` (windows-race.sh sets both), or
  `XO_PROF=cpu` for a program, and compares the same breakdown from a
  known good xo. Check how each xo under comparison was built (`xo
  version -m`: backend, release preset) before blaming the code.
- A measurement needing a quiet machine runs with HEAVY_CLASS=measure
  (above); inside a window the lead opened by hand, it waits with
  `~/xo-lead/agents/quiet.sh wait [LOAD]`.
- Gate before measuring a runtime or allocator change (lead, 2026-10-26:
  a threshold allocator was measured twice and its self-build footprint
  taken before its fixed point from the seed showed a race that crashed
  s1; every number was redone): the fixed point (from the seed when the
  change is in std/native or llrt) and the change's tests pass on the
  exact build you measure, then the `--ab`.
- An arm64 or x86 isel or emitter change runs internal/arm64
  `TestGoldenAsm` in its gate (lead, 2026-10-26: a compare encoding
  change reached main with four stale assembly goldens; `-update`, review
  the diff, commit the goldens with the change).
- A code generation change (isel, emitter, a pass, an intrinsic, a
  crypto or runtime routine the patterns name) runs `xo dev codegen
  test/codegen` in its gate; since 2026-10-27 its default backends are
  arm64, amd64 and llvm, so the x86 wins are checked too (`// amd64:
  "re"` patterns on AT&T text, the host's object format: `callq`,
  `\tjae ` for a bounds check, `%rip` and `@GOTPCREL` addresses). A new
  x86 win adds its pattern, and checks that it fails with the win
  turned off (`XO_PASSES=-x86_...`) or on the parent's xo
  (gotchas/codegen-absent-pattern-never-matches.md).
- Which tool answers what (owner, 2026-10-22): `xo bench --ab` decides
  "is it faster" (real time, interleaved, with an interval; threads,
  waits and I/O included); callgrind (instruction counts, deterministic)
  and `XO_PROF` say where the cost is. A speedup claim needs an `--ab`
  result. For scheduler, lock, spin, I/O and concurrency changes use
  `--ab` only: callgrind serializes threads, so spin loops look free and
  fiber switches are misattributed. For a codegen only change report
  the callgrind count and confirm with one `--ab` run.
- Two builds or two settings, a verdict with an interval: `xo bench --ab
  A [B] [--env-b=K=V] [--max-load=X] -- ARGS` (decision 0340,
  toolchain.md "A/B measurement"): interleaved rounds, median, spread, a
  95% interval for B/A, "within noise" when it includes 1.0. Use it
  instead of a hand rolled loop. While the lead's measurement window is
  open (a future epoch in ~/xo-lead/quiet), only the window's own runs
  proceed: started through heavy.sh with HEAVY_QUIET=1 (heavy.sh passes
  it to the run), or with XO_BENCH_IN_WINDOW=1 by hand; every other run
  waits for the window to close (2026-10-23: two queued `--ab` runs
  without it sat idle through a whole window, then ran under load). The
  load guard (--max-load) applies either way.
- The compiler's own edit-build time: `xo build --backend=arm64
  --phase-times ./xo` in compiler/ prints each phase, its sub-phases
  (insert_rc.plan|read|apply, inline.apply|verify) and the cache's hits
  (decision 0220 addendum stage D, bench/results/fast-builds-2026-10-13.md
  "Stage D"); `XO_PROF=cpu XO_PROF_HZ=500` on the build, then `xo prof`,
  for where a phase spends its CPU. XO_BACKEND_CACHE=off for the full
  build a cached one must equal.
- HTTP callgrind: `MODE=short bench/perf/http/callgrind-docker.sh` for
  gates (2 to 3 minutes); the full run only for what toolchain.md "HTTP
  callgrind: short or full" lists. `ANN=1` keeps per-function counts;
  `xo dev compare [--json] [--top=N] BASE/run NEW/run [SCENARIO...]`
  prints each scenario's instructions a request and the functions that
  changed most (decision 0264 stage 1; cgcompare.py prints the same and
  stays as the cross-check until stage 2's build stamps land).
- Who allocates (or calls anything) per request: run the callgrind with
  `KEEP=1` (bench/perf/http/callgrind.sh keeps OUT/cg.SCENARIO.N), then
  `xo dev compare --callers='halloc$|rs_new_n$' OUT/run plain`: each
  caller's calls and inclusive instructions a request, names as Xo
  writes them (`obj.halloc`, not `xo_f_obj_2ehalloc`; decision 0264).
  Arm64 tail calls (`b`) bill a callee to the caller's caller; for call
  sites by address run with `CGFLAGS=--dump-instr=yes` (both formats
  read).
  An edge marked `[!]` (JSON `"suspect"`) costs over 4x a call what the
  callee's cheapest edge costs: probably a fiber switch billed to the
  interrupted frame (callgrind runs threads one at a time and charges the
  next fiber's work to the call in progress), not work the callee did.
  On 2026-10-22 `rl_copy_n` showed 9,035 instructions a call from
  `H2Conn.fragment` (15.6% of an h2 request) that a list reuse change and
  `xo bench --ab` showed did not exist. Confirm any such cost with
  `xo bench --ab` before acting.
- Which build an xo is: `xo version --json` (or `-v`): commit, dirty
  diff hash, source key, the xo that built it (decision 0264). The bench
  runners (bench/perf/run.sh, llvm/native.sh, conc/run.sh,
  http/callgrind-docker.sh) refuse an xo whose source key is not the
  checkout's (`go run ./scripts/xobuild --key`); XO_BENCH_ANY=1
  overrides.
- `scripts/asmnorm.py FILE`: an `xo build --asm` file without DWARF
  sections and source columns, to check that a source rewrite builds the
  same code and data (gotchas/rewrite-moves-source-columns.md).
  `scripts/std-identity.sh XO REV_A REV_B` runs it over every
  internal/llvm golden and the compiler with std at two revisions (heavy
  lock, about 25 minutes).
- `python3 scripts/gen-rt-sized.py std/rt/sized.go`: rewrites the Go
  backend's checked sized integer helpers (gotchas/checked-arith-inline-budget.md);
  std/rt's TestSizedHelpersMatchGeneric checks them.
- `scripts/fieldcheck.py [--quick]`: after regenerating x25519.xo or
  nistec.xo's field code, interprets the generated Xo against Python big
  integers and checks that a planted dropped carry is caught. The
  generators are Xo programs: `xo run scripts/gen/x25519fe64` (also
  nistecmul4p, keccaklazy for sha3.xo) prints the code to splice in;
  internal/llvm TestFieldGeneratorsInSync fails when a file and its
  generator disagree.
- `xo vet` XO0530 (a field's large payload bound and shared) and XO0531
  (a large parameter only stored while each caller passes its last use),
  decision 0263.

- `scripts/sign-macos.sh [--notarize] FILE...`: Developer ID signing
  (XO_SIGN_IDENTITY) and notarization (credentials from the environment,
  sourced in a subshell) of macOS binaries; internal/arm64
  TestCodesignAcceptsLinkerOutput checks that codesign accepts
  compiler/macho's output (code-signing.md).
- `scripts/sign-windows.sh FILE.exe...`: Authenticode signing of
  Windows executables with Azure Artifact Signing through jsign 7.5
  (JSIGN_JAR, JAVA; the six AZURE_* credentials from the environment,
  sourced in a subshell; billed per signature), then prints each file's
  certificate subjects and whether the RFC 3161 timestamp is there;
  internal/llvm TestWindowsCertTableAppends checks that a signed
  layout still parses (code-signing.md "Windows").
- `scripts/cache-churn.sh [--dir DIR] [--since UNIX] [--live-min N]` (2026-10-24, decision 0365's churn measurement): the back-end cache's store churn by hour and for the last 24 hours and the whole period, from DIR/store-log and DIR/prune-log: GB stored, GB pruned, distinct ids, GB stored by retired ids (an id with no store in the last N minutes, default 60) and their share. Read only; `--since` defaults to 2026-10-24 about 20:45 local. Run it on the cache's machine, e.g. Sunday morning for the owner's review.

## Coordinating (scripts/lead/README.md)

- Gates: `HEAVY_CLASS=gate ~/xo-lead/agents/heavy.sh NAME CMD...`: no slot,
  no queue; starts when 55% of memory is free (HEAVY_GATE_MIN_FREE) and
  fewer than 3 gates run (HEAVY_GATE_MAX).
- Heavy runs: `~/xo-lead/agents/heavy.sh NAME CMD...`. Slot order:
  pushq's runs, then gates, then `HEAVY_CLASS=bulk` runs (measurements,
  profiles, suites). While `~/xo-lead/merge-first` exists, gates are the
  bare minimum and other checks go to `~/xo-lead/notes/deferred.md`
  with their commands (CLAUDE.md "merges before testing").
- Pushes: `~/xo-lead/agents/pushq.sh WORKTREE [--base SHA] [-- GATE...]`.
- Status: `~/xo-lead/agents/board.sh set|claim|show`, instead of status
  messages.
- Numbers: `~/xo-lead/agents/adr-claim.sh SLUG WHO` for a decision
  number, `~/xo-lead/agents/code-claim.sh PREFIX WHO [WHAT]` for a
  diagnostic code (PREFIX is the family, `XO09` or `XO13`): each takes a
  lock, reads origin/main and its claims file (~/xo-lead/adr-claims.txt,
  ~/xo-lead/code-claims.txt), records the next free one, and prints it.
  Claim before writing the code into a branch; a claimed code is never
  given out again, even if its branch is dropped (core 10: a code keeps
  its meaning). A code used twice on main fails -short
  (internal/repotest explainCodes and TestSpecListsEveryCode).
- `XO_CLI` pinned to a worktree's bin/xo goes stale when that worktree is
  rebased (pushq does): the Go tests then fail at once with `xotool:
  XO_CLI=... is stale` and both keys and commits, instead of running newer
  std and goldens against the older compiler (Xo lang 5 lost a 60 minute
  suite to that, 2026-10-21). Rebuild (`go run ./scripts/xobuild` in that
  tree) or, for a deliberate cross version run, `XO_CLI_STALE_OK=1`.
  Every such run's log starts with `xotool: xo PATH (key ..., commit ...);
  tree key ...`: check it when a run's failures look unrelated.
- Checking a compiler edit: `go run ./scripts/xocheck [--no-gc] DIR...`
  runs `xo check` with an xo that reports what a fresh build would (the
  tree's current bin/xo, else a cached build whose checker sources,
  internal/xotool CheckerDirs, are the tree's; --no-gc adds compiler/ssa)
  and the tree's std, building only when there is none. Never check with
  an older worktree's bin/xo: its checker and std differ (2026-10-21). A
  directory inside std is refused with that advice (2026-10-24): std's
  modules are checked as a program's dependencies, so `xocheck
  std/native/llrt` printed 7,530 "module native/llrt/raw not found"
  errors; check a program that uses the module.
- Output identity (a change that claims no output change): `go run
  ./scripts/identity [-targets T,...] [-backend B] [-step N] [-std DIR]
  [-out DIR] BASE NEW [DIR...]`. BASE and NEW are xo binaries or git
  revisions (built in a temporary worktree, removed after; the base is the
  change's parent, not main). Every -step-th golden and compiler/xo by
  default, with one std for both, the same output names, every cache off,
  a private backend cache for every xo it starts (old xos' prune), each
  pair deleted once compared and a differing one kept in OUT/keep; Mach-O
  compared by disassembly, ELF byte for byte. Exit 1 when one differs or
  NEW fails to build one. Run it in a bulk slot (`HEAVY_CLASS=bulk
  heavy.sh`). Last verified: 2026-10-21 (the same xo on both sides: 12
  compared, 0 differing).
- Gate scripts: `. ~/xo-lead/agents/gatelib.sh` (scripts/lead/gatelib.sh,
  installed by install.sh), then `gotests PKG 'TestA|TestB'` for the
  proving tests (exactly those names, -v, one `--- PASS` line each) and
  `no_tests_check FILE` after any other go test step: a `-run` filter that
  matches nothing, or only some names, passes otherwise
  (gotchas/go-test-run-matches-nothing.md). Last verified: 2026-10-21.
- Rebase or merge conflicts: `~/xo-lead/agents/resolve-addadd FILE...`
  (or `python3 -I ~/xo-lead/agents/resolve-addadd.py FILE...`; source
  scripts/lead/resolve, installed by scripts/lead/install.sh) merges each
  conflicted file again from the index: a .go file by top level
  declaration (imports united, entries both sides added to one table
  merged), any other file only where both sides added lines at one place
  (and spec core's `xo vet` row codes). Whatever changed on both sides is
  refused: the file is left as is and named (exit 1), to resolve by hand.
  Name the files one by one under zsh, then read `git diff` and
  `git add` (it stages nothing). Last verified: 2026-10-21.

## Coming

- `dbg` in pure and no-GC code (0265), vet hints for
  copy-modify-store (0266).
