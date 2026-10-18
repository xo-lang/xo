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
  rt.int_wrap intrinsics (spec/std.md). Prefer them over `rt.int_wrap`
  in new code.

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
- `scripts/arm64-identity.sh`, `scripts/docker-goldens.sh`,
  `scripts/intel-goldens.sh XO HOST REMOTE_DIR WORK` (the Intel laptop;
  owner rules in the lead's notes: one new directory, nothing deleted),
  `scripts/macho-link-compare.sh`.
- `scripts/gate-http.sh OUTDIR [STEP...]`, `scripts/fuzz-http.sh OUTDIR
  [SECONDS]`.
- Test programs started by Go tests run in their own process group and
  are stopped (TERM, then KILL) when the test ends (testtmp.Guard).

## Measuring

- HTTP callgrind: `MODE=short bench/perf/http/callgrind-docker.sh` for
  gates (2 to 3 minutes); the full run only for what toolchain.md "HTTP
  callgrind: short or full" lists. `ANN=1` keeps per-function counts and
  `bench/perf/http/cgcompare.py BASE/run NEW/run` prints the top changed
  functions (to be folded into `xo dev compare`, decision 0264).
- `scripts/fieldcheck.py [--quick]`: after regenerating x25519.xo or
  nistec.xo's field code, interprets the generated Xo against Python big
  integers and checks that a planted dropped carry is caught.
- `xo vet` XO0530 (large copies).

## Coordinating (scripts/lead/README.md)

- Heavy runs: `~/xo-lead/agents/heavy.sh NAME CMD...`.
- Pushes: `~/xo-lead/agents/pushq.sh WORKTREE [--base SHA] [-- GATE...]`.
- Status: `~/xo-lead/agents/board.sh set|claim|show`, instead of status
  messages.

## Coming

- `xo run --watch` and `xo test --watch` (decision 0267), `xo dev
  compare` (0264), `dbg` in pure and no-GC code (0265), vet hints for
  copy-modify-store (0266); std moving from rt.int_wrap to the Int word
  methods (0260 step 3, after seed 0.21.0).
