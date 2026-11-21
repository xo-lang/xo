# 0738: Std compiled once for the LLVM backend too

Status: Proposed (Xo Lang 1, 2026-11-21, task #607; the owner chose the
direction, xw decision #250; main only, not 0.55.x)

## Context

Every `--backend=llvm` build is a whole program: compiler/xo sepenv.xo
refuses the separately compiled std when the backend is not the host's
native one ("std variant: not used (another target or backend)"), so
std's functions are emitted as LLVM IR and compiled by clang in every
program that reaches them.

Task #593 ranked the 861 LLVM goldens by build time at 63b5d3c46
(linux/arm64 host 2, three at a time, ~/xo-lead/notes/xo-lang-1/p593):
clang is 9,043 s of 9,679 s. No superlinear pass is left after decision
0731. 696 goldens spend 2 to 10 s in clang (3,530 s); 126 spend 30 to
100 s (4,989 s, 55%): every http, tls and net program, and small ones
that reach all of crypto (list_swap_guard, 20 lines, 43.8 s). Such a
program is one module of about 690,000 lines of IR (the median of the
126) holding about 4,000 std functions; `clang -ftime-report` on one
spends its time evenly in code generation (instruction selection 21%,
register allocation 15%, scheduling 11.5%, InstCombine 11%).

Reach, measured (task #607, p607/reach1.out: the kept IR of each of the
126, every function counted by its owner):

| Owner | IR lines | Share |
|---|---:|---:|
| plain std functions (the same in every program that reaches them) | 76,150,868 | 88.6% |
| std generic instances (per program) | 7,157,302 | 8.3% |
| the runtime | 1,637,318 | 1.9% |
| std clones (specialize_args, #once) | 635,884 | 0.7% |
| the program's own functions | 383,263 | 0.4% |

Per golden the plain std share is 71.3% to 92.0%, median 88.8%. If
clang's time follows lines, 4,424 s of the 4,989 s are std's and 566 s
remain. That is an estimate from line counts, not a measurement: stage
0 below measures it.

The native backends have had this since decision 0388: std compiled
once per variant into an indexed store, with exported facts, and a
program compiles its own functions, its generic instances and its
specialize_args clones against it.

## Decision

1. Std is compiled once per variant for the LLVM backend: std built
   alone, every std function a root (0388's XO_STD_ROOTS), emitted as
   LLVM IR, split into modules, compiled by clang with
   `-ffunction-sections -fdata-sections`, and kept in the backend cache
   as std's LLVM objects. The variant key is the native one's (xo's
   identity, std's sources, target, conc, test mode) plus what changes
   clang's output: the optimization level, debug information, and
   clang's identity.
2. A program's LLVM build takes 0388's facts and export records as the
   native build does (they are made from SSA and the checker, before
   any backend), builds SSA for the program's functions, its generic
   instances and its clones only, and emits those. A std function it
   calls is a `declare`; std's functions get hidden external linkage
   and the names the store's objects define, where they are `internal`
   today.
3. The link is clang's, of the program's object with std's objects,
   with dead stripping (`-dead_strip` on darwin, `--gc-sections` on
   ELF), which replaces the whole program's DCE and hollow for std.
4. Small std functions stay inlinable: the bodies 0388 exports for the
   native inliner are emitted into the program's module as
   `available_externally` definitions, LLVM's form for a body that may
   be inlined and is never emitted.
5. Whole program stays for `--release` (decision 0220, as native),
   for `--race` and the sanitizer builds (std would need an
   instrumented variant each), for wasm, and for a target that is not
   the host's until its variant exists. `XO_SEPCOMP=0` turns it off.

Stages, each measured before the next:

0. Oracle, no compiler change: std's IR alone through clang once (the
   store's cold cost and size), and one slow golden's IR with std's
   plain functions made declarations, compiled and linked against that
   object with dead stripping (the warm cost: clang and the link). The
   stage ends the work if the warm build is not under about a quarter
   of today's.
1. std's LLVM objects: the cache kind, `xo dev stdcache --backend=llvm`.
2. The program's side: emit against the facts, the link, behind
   `XO_SEPCOMP_LLVM=1`; the LLVM goldens pass both ways.
3. `available_externally` bodies; `xo bench --ab` of the scorecard's
   LLVM rows against whole program.
4. On by default for dev builds; the tier's llvm golden step measured
   before and after.

## Consequences

- A warm LLVM build of a program that reaches http or crypto compiles
  its own IR (median 11% of today's lines for the 126) and links.
  Expected from the line counts: the 126 goldens' clang time from 4,989
  s toward 566 s plus the links, and the store's one build per variant.
- Less optimization across the std boundary: one module with
  `internal` linkage lets clang inline, specialize and drop arguments
  across all of std. The native default build shows what that costs
  when nothing is exported for it (task #511: fault defer 1.725 of LLVM
  against 1.018 whole program). Item 4 keeps inlining; interprocedural
  constant propagation and dead arguments across the boundary are lost.
  Release builds are unaffected.
- The output differs from today's whole program build by design; the
  goldens compare behavior, so they hold, and timing baselines of dev
  LLVM builds are taken again.
- More link work: std's objects and their DWARF are read at every
  link. 0388 measured this for the native linker (most of the cost was
  DWARF) and built an indexed store; clang's linker has no such store,
  so stage 0 measures the link first.
- One more cached artifact per variant, hundreds of megabytes with
  DWARF; the backend cache's prune covers it.
- A sanitizer or race golden gains nothing until its variant exists.

## Alternatives

- ThinLTO with a shared cache (`-flto=thin` with the linker's cache):
  keeps cross module inlining, and the cache reuses a module's code
  generation when its imports are the same. Rejected as the first
  step: a hit depends on each program's import decisions, so reuse
  between different programs is not something xo controls. Worth an
  oracle beside stage 0, since it needs no facts and no new linkage.
- Caching clang's output per split module: the test binary has this
  already (xotool EnableTestClangCache). A whole program's IR is
  pruned and specialized per program, so two programs share no
  byte identical module.
- Running fewer goldens on LLVM, or not the ones that reach crypto
  through internal.crypto_op: the owner chose reuse (xw #250); it also
  leaves users' LLVM builds as slow as they are.
- Linking the native std store into an LLVM build: the two backends'
  calling conventions and unwind tables differ.
