# 0745: LLVM's role, and the native backends better than LLVM

Status: Accepted (owner, 2026-11-22, xw decision #279; task #715).

## Context

Xo has two ways to machine code: its own native backends (arm64 and
x86-64, compiler/arm64, with Xo's assembler and linker) and the LLVM
backend (compiler/llvm, through clang). The native backends are the
default on the targets they serve. LLVM came first on several targets,
carries `--race`, and is what native output is compared with: rows such
as bench/perf/portable are read as native instructions and time against
LLVM's (decision 0727).

Three questions kept coming up one task at a time: whether a feature
must work on LLVM too, how much of the test time LLVM's goldens may
take (the linux LLVM golden step ran 3,392 s in tier 920, task #593),
and what the native backends are aiming at.

## Decision

1. **LLVM stays a supported backend.** It is the bring-up path of a
   new target (riscv64: decision 0571, then 0645), the backend of
   `--race` (decision 0241's addendum, and 0273), and the reference the native
   output is checked against.
2. **Compliance.** Every language and runtime feature keeps working on
   LLVM. A feature is not done until its LLVM lowering and its goldens
   pass there.
3. **LLVM is tested less.** The 2-hourly continuous tier runs a
   rotating fifth of the LLVM goldens, so every golden runs at least
   twice a day; the 8-hour tier runs the full set. Per push nothing
   changes: a change to compiler/llvm gates on the llvm step.
4. **The goal: the native backends better than LLVM on every native
   target**, by four measures: compile time, peak memory of the
   compile, size of the code (`__text` or `.text`), and run speed.

## The measures

For each native target (darwin/arm64, linux/arm64, linux/amd64,
windows/amd64, and the others as they come), native against LLVM on
the same programs:

| Measure | How it is read |
|---|---|
| Compile time | `xo build --phase-times`, and `xo bench --ab` of the build for a claim |
| Peak memory of the compile | max resident set of `xo build` (`xo bench --ab` max_rss_kb) |
| Code size | the text section's bytes; `xo dev size --by-cause` names what moved |
| Run speed | `xo bench --ab` on host 1 for linux/arm64; instruction counts explain, they do not prove (decision 0340) |

Where each target stands on each measure is a results file to write
(bench/results), not a statement of this decision. Run speed is the
measure the first steps aim at:

- Task #712: profile-guided inlining of hot std call sites in the
  default build.
- Task #713: a native loop optimizer and auto-vectorizer (arm64 NEON
  first, then x86-64 SSE2, AVX2 behind CPU detection; floats stay
  strict, decision 0287).

Each gets its own decision before it is built.

## Consequences

- A compiler or runtime change is reviewed for its LLVM lowering as
  for its native one; a native-only feature is a gap to close, not a
  place to stop.
- A failure on an LLVM golden can now be up to a rotation old when the
  continuous tier finds it (at most about half a day); the 8-hour tier
  and the bisect of main name the commit, and its owner fixes forward
  as for any other failure on main.
- Comparisons with LLVM stay like for like (both with their default
  caches) and name their source file, as every claim does.
- "Better than LLVM" is claimed per target and per measure, from a
  committed results file, never in general.

## Alternatives

- Drop the LLVM backend once the native ones lead: loses the bring-up
  path, `--race` and the reference.
- Keep testing LLVM as much as the native backends: about an hour of
  each tier for a backend that is not the default.
- LLVM as the default for release builds: the native backends would
  stop being measured where it matters.
