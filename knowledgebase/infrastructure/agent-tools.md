# Agent tools: what exists, and when to reach for it

One index of the tools built for agents working on Xo (owner,
2026-10-18: make sure every agent knows them). Each entry names where
its full documentation lives. Last verified: 2026-10-18 (main 41a6afe2).

## Finding code (decision 0261)

- `xo def NAME`, `xo refs NAME`, `xo type FILE:LINE:COL`, `xo doc NAME`
  (a type's methods), `xo symbols [--all]`; `--dir=DIR`, `--json`. Use
  them instead of grep for declarations, callers, types, and to check
  that a method exists before calling it (`Map.update` existed while
  agents guessed `update_or`, 2026-10-18).

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

## Coordinating (scripts/lead/README.md)

- Heavy runs: `~/xo-lead/agents/heavy.sh NAME CMD...`.
- Pushes: `~/xo-lead/agents/pushq.sh WORKTREE [--base SHA] [-- GATE...]`.
- Status: `~/xo-lead/agents/board.sh set|claim|show`, instead of status
  messages.

## Coming

- `dbg` in pure and no-GC code (0265), vet hints for
  copy-modify-store (0266).
