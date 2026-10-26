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
  for one module, 2026-10-21); `--dir=DIR`, `--json` or `--text` (text at a terminal).
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
  `xo refs --testdata` checks them too (the xo checkout: 11.5 s and 6 GB
  against 4 s, so run it through heavy.sh). Refs of a std name checks
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
  which needs implementation edges). Last verified: 2026-10-25.
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
  turns on the one a target leaves off (hollow on darwin/amd64, 2026-10-25);
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
  On a 99-program sample (2026-10-26, impl-file sites included): 579
  uncovered by the scan, 945 by the edges, and 365 the scan credits that no
  edge reaches, 65 of them interface methods (a call through an interface
  resolves to the interface's method, never to the implementation). The
  rest are bare-name collisions, the scan's known limit and not a corpus
  gap: `prelude.Buf.push` has no reference while `prelude.List.push` has
  7,553, `prelude.Bytes.contains` none while `prelude.Str.contains` has 525,
  `prelude.Duration.abs` none while `prelude.Int.abs` has 4 (checked with
  `xo refs`). A line the edges contradict is worth a look; a declaration
  the edges cannot reach is not by itself proof of a missing corpus.
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
- `scripts/progen-batches.sh`, the amd64 progen runner, TestArithOracle.
- `scripts/codesize.py BINARY`: where a darwin/arm64 build's `__text` goes (functions, modules, generic copies, instruction kinds, whole value word copies, release runs, constant addresses); bench/results/xo-code-size-2026-10-18.md.
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
  xo in place). Start a measurement only on the lead's go in a window,
  never automatically when the load drops: two runs started that way
  overlapped another agent's window.
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
- A measurement needing a quiet machine waits with
  `~/xo-lead/agents/quiet.sh wait [LOAD]` inside the lead's window.
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
