# Agent tools: what exists, and when to reach for it

One index of the tools built for agents working on Xo (owner,
2026-10-18: make sure every agent knows them). Each entry names where
its full documentation lives. Last verified: 2026-10-18 (main 41a6afe2).

## Finding code (decision 0261)

- `xo def NAME`, `xo refs NAME`, `xo type FILE:LINE:COL`, `xo doc NAME`
  (a type's methods), `xo symbols [DIR] [--pub]` (the directory's own
  declarations; at a root with no .xo files of its own it names the
  directories below that have them); `--dir=DIR`, `--json` or `--text` (text at a terminal).
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
  `--dir` may be a workspace or repository root, but a module there whose
  `use go` or `use c` binding is missing stops the whole check: the miss
  then says so (the xo checkout's root: examples/realtime-control's
  `use c`); give `--dir` a module's directory. Last verified: 2026-10-21.
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

- `XO_PASSES=-name,...` turns optional passes off; `xo dev bisect-pass
  DIR` finds the first pass that changes a program's output.
- `reduce-diff.py --crash` shrinks a failing program; property tests
  keep the failing program, print the build's output, the culprit pass,
  and the reduced program.
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

- `xo test --std-module=MODULE` runs a std module's `_test.xo` files with
  private access (decision 0262); internal/stdtest runs every std module
  that has tests. Tests under std/ and compiler/ live only in `_test.xo`
  files.
- `go run ./scripts/stdindex` after changing std/prelude.xo: spec/std.md
  Appendix A lists every prelude function and method by type (look there
  for a method's exact name and signature); TestStdIndexCurrent fails
  while it is stale.
- `go run ./scripts/stdgen` after changing a std type (a struct, enum,
  newtype, or interface in std/*.xo) or a std function's signature:
  regenerates std/rt/std_gen_M.go, the Go backend's declarations of
  module M's std types, and std_sigs_M.go, which makes `go vet ./std/rt`
  name a hand written function whose signature drifted (toolchain.md),
  one pair per module since 2026-10-21, so two pushes that change
  different modules do not conflict there; the -short `TestStdGenCurrent`
  fails until it is run. Never edit those files by hand, and resolve a
  conflict in one by regenerating.
- `scripts/gate.sh WORKTREE STEP...`: an agent's gate as named steps.
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
  a line a cycle and `.xo/watch/last.json` (decision 0267).
- Test programs started by Go tests run in their own process group and
  are stopped (TERM, then KILL) when the test ends (testtmp.Guard).

## Measuring

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
