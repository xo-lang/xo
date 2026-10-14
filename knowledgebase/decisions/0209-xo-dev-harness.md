# 0209: Test harnesses as an `xo dev` subcommand written in Xo

Status: Accepted (owner, 2026-10-13); designed 2026-10-13 (section "Design"), not implemented. Queued after decision
0195's zero-C work, as part of the tooling hardening.

## Context

Three Python scripts (scripts/selfhost-goldens.py,
selfhost-gogoldens.py, elf-goldens.py) build the golden programs with
xo1, run them, and compare output. They overlap, behave differently by
platform (a text mode read turned show_table's raw CR into LF,
gotchas/python-text-mode-want-cr.md), and need Python. After decision
0178 stage 7 the Go compiler packages are deleted (the test helpers
stay), so Go tests are not the long term home for Xo's own tooling.

Why Python: the first script (selfhost-goldens.py, 2026-10-11, the
decision 0158 switch-over experiment) was scaffolding to drive an
external compiler binary while the Go tests called the Go compiler in
process; the other two copied it. No decision chose Python. The Go
tests already hold the same comparison logic (TestGoldenRun,
TestGolden, stableStderr, dropGoFrames) and already accept an external
compiler for the want.txt goldens (`XO_GOLDEN_WANT=1
XO_GOLDEN_WANT_XO=<xo1> go test -run TestGoldenWant ./internal/llvm`,
used for the Windows runs), which is the starting point.

## Decision

The harnesses become a hidden developer subcommand of xo, written in Xo
(for example `xo dev goldens --backend=arm64 <dir>`, later the replay
recorder, the ELF and Mach-O checks, and the Windows batch steps), with
byte exact comparison by default and code shared with `xo test`. One
thin Go test per harness calls it, so the existing tiers and batches
keep running it. The Python scripts run beside it until both agree on a
full batch, then they are deleted.

The same work gathers xo's hidden developer commands (cmd/xo/main.go:
lexdump, astdump, typedump, ssadump, asmdump, llvmdump, gendump, rtpack)
under `xo dev` in xo1: `xo dev goldens`, `xo dev dump
lex|ast|types|ssa|asm|llvm|go`, `xo dev rtpack`, `xo dev record`; xo1
gains the dumps it lacks today (ssadump and llvmdump, the two commands
the switch point B gate exempted). The Go compiler's dump commands go
with stage 7's deletion.

## Consequences

- The harness dogfoods Xo's std (files, processes, text) on every run
  and works on every host xo1 runs on, without Go or Python.
- A std bug can break the harness itself; running beside the old scripts
  until they agree catches that before the switch.
- `xo dev` is not part of the user facing CLI surface in spec/core.md
  (hidden, like the dump commands).

## Alternatives

- Go tests only: rejected; ties Xo's tooling to the Go toolchain and is
  not runnable outside the repository.
- Keep the Python scripts: rejected; platform differences and three
  overlapping implementations.

## Design (2026-10-13)

Phase 1 of the work: measured first
(bench/results/xo-dev-harness-2026-10-13.md), designed against those
figures. Implementation (phase 2) starts after decision 0215's deletion
of the Go compiler is merged, and lands in selfhost/ and std first.

### What the measurements say

- The golden harnesses are bound by xo1 and the C toolchain, not by
  Python: on the Mac the arm64 set (179 programs) takes 49.12 s wall,
  the LLVM set 65.24 s, the Go backend set (136) 19.85 s. Of the arm64
  builds' 109.6 s, cc is 56.6 s and the check phase 20.7 s, about 96
  percent of it std checked again for every program (109 ms for a
  program with no imports, median 113 ms). One program,
  huge_frame, is a third of the arm64 run's wall (15.80 s build).
- The selfhost recording's floor is one input: selfhost/xo. Its Go
  arm64 emitter item alone is 653.9 s of the 2,020 s recording on
  r8g.8xlarge, its SSA item 296.3 s, its LLVM item 170.3 s; the top
  level tests run one after another, so these add up, and more cores
  cannot help (the results file ranks the items). Decision 0217 takes
  these replays out of the default runs (below).

### Command surface

Hidden (not in `xo help` or spec/core.md), in xo1 (selfhost/xo), one
dispatcher `xo dev <sub>` beside rtpack:

- `xo dev goldens [--backend=arm64|llvm|go|wasm] [--target=OS/ARCH]
  [--as=..] [--ld=..] [--cmd=build|run] [-j N] [--json] [--force]
  [--only=SUBSTR,...] [--keep=DIR] DIR...`: build each program
  directory under DIR (a directory with .xo files and a want.txt, or a
  `runwant/<name>.txt` beside its parent, the layout decision 0215
  left), run it with its args.txt, env.txt, stdin.txt, XO_LEAKCHECK=1
  on native backends, and compare. Exit 0 when all pass; the text
  report is the scripts' (`N programs: P pass, F fail`, reasons
  grouped, first failures shown); `--json` prints one object per
  program and a summary for the Go wrapper. `--run-in=docker:IMAGE`
  (later, for ELF on a Mac) runs the binaries in one container as
  elf-goldens.py does; on a Linux arm64 host they run directly.
- `xo dev dump lex|ast|types|ssa|asm|llvm|go [--each] [flags] PATH...`:
  the component dumps the selfhost tests compare (today the separate
  selfhost/<component> mains), callable from xo1: each component's
  main body becomes a `pub fn dump_main(os, args) -> Int` in its
  module, the component's own `fn main` calls it, and `xo dev dump`
  dispatches to it. ssa and llvm are the two dumps xo1 lacked at
  switch point B.
- `xo dev rtpack`: the current hidden `rtpack`, moved (the old name
  stays an alias until the release scripts change).
- `xo dev errorcheck DIR...` (decision 0217): check each file and
  compare its diagnostics with the expected ones written as comments in
  the source (`// ERROR "regexp"` on the line, as Go's test/
  `// errorcheck`); a missing or an extra diagnostic fails.
- `xo dev codegen [--backend=arm64|llvm] DIR...` (decision 0217): build
  each file to assembly or IR and check per function assertions written
  as comments (`// arm64: "pattern"`, `// arm64: -"pattern"` for must
  not appear, as Go's test/codegen), never whole files.
- `xo dev record [--merge] KIND...`: writes
  internal/selfhost/testdata/gogoldens from the Xo side, only for the
  front end kinds decision 0217 keeps (lexdump, astdump, format,
  typedump); the back end recordings are not refreshed and the ones no
  test uses are deleted at the end of phase 2.

### What decision 0217 changes

Decision 0217 (owner, 2026-10-13) verifies the compiler by behavior:
program output goldens on every backend, the fixed point, error check
tests, codegen pattern tests, progen across backends, and Xo unit tests;
the back end and mutation dump replays are skipped by default. The
recording's floor items (selfhost/xo in the Go arm64 emitter 653.9 s,
SSA 296.3 s, LLVM 170.3 s) leave the default runs with them. What
remains per change, ranked (results file, "Re-ranked after decision
0217"): the xo1 rebuild and the fixed point (164 to 420 s per build),
the program output goldens (about 134 s on three backends on the Mac),
TestSelfhostWasm (98.45 s), and the front end replays and Xo unit tests
(about 27 s). So the harness's weight moves to the goldens, which
become the main proof, and the levers below are aimed at them and at
the xo1 build; the error check and codegen runners share the same
worker pool, cache, and report.

### Lever 1: std loaded and checked once per run

A run checks std once per worker, not once per program: each worker
keeps a checker whose std modules (the prelude, every std and native
runtime module any program of its share imports, keyed by the check
options: native or not, the target) are loaded and checked on first
use, and checks each program against a copy of that state
(`check.Checker.fork_std()`: the module table, the checked std
declarations, and the type tables shared read only; the program's root
modules and diagnostics fresh). The output of a forked check must be
byte identical to a fresh `new_checker` (the existing selfhost check
differential gains a fork mode that checks every run both ways). One
std state shared by all workers is a later step: it crosses threads, so
its reference counts go shared and atomic (decision 0058); measure that
cost before doing it. The same reuse later applies to the SSA and
insert_rc work on std functions (insert_rc is 12.0 s of the arm64 set,
9.57 s of it huge_frame), keyed per function.

Gate: the arm64 set's check phase goes from 20.7 s summed to about the
programs' own share plus one std check per worker, with no output
change.

### Lever 2: unchanged work skipped by input hash

A program's result is cached under a key: SHA-256 of the harness
version, the xo1 binary's own key (its BinaryKey: selfhost and the std
modules it uses), the backend and flags, the host and target, every
file of the program directory (and its runwant file), the std tree (one
hash per run, computed once), std/llrt and the runtime pack, and the
args, env, and stdin files. The entry (`<user cache>/xo/dev/goldens/
<key>`) is written only on a pass and holds the output compared, so a
cached pass replays as a pass with no build. `--force` (and the full
tier, and every release run) ignores the cache. Caches that changed
checker output when stale, `.xo/gobind` and `.xo/cbind`, are not
trusted: the harness points XO_GOBIND_DIR and XO_CBIND_DIR at a
directory of its own per run (and their contents are in the key if a
program uses them). Program inputs are listed through `git ls-files`
when the tree is a git checkout, as internal/selfhost's walk now does
(gotchas/replay-local-files-in-main-checkout.md), so local untracked
files neither change keys nor get compared.

This lever does not help a compiler change (the xo1 key changes, every
program rebuilds); it helps reruns after a fix, golden edits, and std
edits outside what xo1 uses. For the selfhost replay tiers the same key
on a unit (the component binary's key and the unit's inputs) skips a
dump whose binary and input are both unchanged, which matters for the
front end replays decision 0217 keeps and for `XO_SELFHOST_DUMPS=all`
audits, and the same unit key skips duplicate units within a run
(selfhost/cli/modules/xomod repeats cli/modules, about 130 s in the
arm64 emitter).

### Lever 3: in-process parallelism

`-j N` workers (default: the host's cores, at most 8, and one per 2 GB
of free memory for native builds), each a task (std/task) that takes
the next program, largest first: the order is by last run's build time
from the cache directory, else by source size, so huge_frame starts
first. Each worker builds in process (the checker, SSA, emitter calls
xo1 already makes, no xo1 subprocess per program), then runs the
program as a child (`os.proc.cmd(...).dir(d).env(...).stdin(...)
.output()`). Needed from std first: a per child timeout (the native
Proc has none yet, selfhost/xo/test.xo), so a hung golden fails
instead of hanging the run (elf-goldens.py uses 60 s). For the selfhost
tiers (the kept front end replays, and the back end dumps in a
`XO_SELFHOST_DUMPS=all` audit) the same lever is per unit: the arm64,
SSA, and LLVM dumps of a large component split by function (the dumps are already cut into
per function pieces for comparison), and the top level tests stop
running one after another (one pool across tests), so the floor becomes
the longest single unit instead of the sum of each test's longest.

### Byte exact comparison

stdout is compared byte for byte with want.txt read as bytes (no
newline translation, gotchas/python-text-mode-want-cr.md). stderr goes
through the one documented filter the Go tests and the scripts use
(timestamps to `<time>`, indented frame lines, `crash bundle:` and
`xo leakcheck:` lines dropped; the Go backend set also drops Go frame
pairs), written once in Xo on bytes and shared with `xo test`'s output
code; nothing else is normalized. The leak check (0 live allocations
on a 0 exit) is checked separately, as now. A mismatch prints a unified
diff of the first lines that differ and, with `--keep`, writes got and
want files.

### The Go test wrapper

One thin Go test per harness (internal/repotest or a new
internal/devgoldens), so `go test` tiers and the EC2 batches keep
running it: it finds xo1 (selfdriver, or $XO1), runs `xo1 dev goldens
--json` with the harness's directories and flags, and fails with the
failing programs. -short runs every 25th program with the cache on;
the default (merge) runs all with the cache on; the full tier and
releases pass `--force`. The wrappers hold no comparison logic.

### New tests are written in Xo

Owner agreed convention: inline `test` blocks next to the code for
small self contained functions (the stderr filter, the key builder,
the batch planner); `_test.xo` files for substantial suites and shared
helpers (helpers whose signatures take `testing` need `_test.xo`). New
compiler and runtime tests are written in Xo (`xo test`), not only as
Go comparisons.

### Switch over

1. `xo dev goldens` lands with its Go wrappers; the Python scripts stay.
2. Both run on the same commit (arm64, llvm, go on the Mac; ELF on a
   Linux arm64 host or with Docker) until both report the same pass
   and fail set on a full batch, by program name, with the before and
   after figures for each lever in the results file.
3. Then scripts/selfhost-goldens.py, selfhost-gogoldens.py, and
   elf-goldens.py are deleted, with their callers
   (scripts/selfhost-switchover.sh, scripts/xo1-host-check.sh), the
   toolchain.md lines, and internal/diag's TestScriptsReadWantBytes
   pointed at the new command.
