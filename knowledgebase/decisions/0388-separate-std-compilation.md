# 0388: Std compiled once to objects and export data; an edit compiles only the program's modules

Status: Accepted (owner, 2026-10-27: "Approved, start"). Proposed by Xo Lang 1 the same day for
milestone 0.34.0, "edit rebuilds at Go's speed", criterion 2; first written as the 0.33.0 edit-rebuild criterion's
structural option, from decision 0382's measurements; the oracles and the DWARF decision below
were added the same day)

## Context

A real body edit of llvm-fib, every cache warm, darwin/arm64, quiet machine (2026-10-27,
XO_CHAIN_KEYS=1 with 0382 stage 3): 274 ms of wall. Its phases run in sequence on the build's
thread, so their sum is the critical path: check 54.4, the SSA build 45 (fe.keys 8, fe.read 7.4,
fe.build 22), the middle passes about 75 (prune_unwind, cln, specialize_args, insert_rc, inline,
dce, inline.verify), the back end about 54 (fn_keys 8.4, emit 8, hollow 18, assemble 7.8, link
11.6), and about 20 outside every phase (process start 3, the key 1, the stores after the link 7,
the exit 9). The HTTP server: 564 ms.

Go's twin of the same program, the same kind of one-function edit, warm build cache: 102 ms
(`go build -debug-actiongraph`, n=5): the edited package compiles in 9.4 ms, the link takes 49.5,
every dependency is a cache check (the action graph spans 80.5 ms, the checks run in parallel),
and about 20 ms is the go command's start. The HTTP server's twin: 246 ms.

The difference is structural. Go compiles the edited package against its dependencies' export
data and links their objects. Xo checks the whole program (std's 195 files are parsed, declared
and resolved on every build: about 38 ms of the check's critical path is std's), builds SSA for
every function, and runs every whole-program pass over all of them (1,300 functions for fib);
decisions 0220, 0283, 0380 and 0382 made each pass's per-function work a cache hit, but every pass
still computes keys, reads entries and decodes over the whole program. The oracle runs of
2026-10-27 (stubs that skip a phase, output ignored) bound what more per-pass caching can buy at
about 35 to 50 ms of the 274 (bench notes: o497, the ceiling table): enough for the 0.33.0
criterion (230 ms, owner 2026-10-26) only with decision 0380 stage 3, and not for 200 or for
anything near Go's 99.

## Decision

1. Std (and each library module a program imports) is compiled once per xo identity, target and
   build flags into an object and export data, kept in the back-end cache. The export data holds
   what a module importing it needs to check and lower its own code without std's sources: the
   checked declarations (types by stable names, signatures, effects, constants), the bodies of the
   functions the inliner may take (Go exports its inlinable bodies the same way), and the facts the
   whole-program passes read of a callee: may_fault and cancel bits, catches, borrowed parameters,
   immortal results, handed-unique facts (0322), result bounds, constant results.
2. An edit rebuild checks, lowers and emits only the program's own modules against that export
   data, then links their objects with std's. Generic std functions instantiated with the
   program's types, and specialize_args clones of std callees, are compiled in the program's
   module (as Go stencils its instances per package).
3. Dead code is removed at the link (per-function sections or subsections, the linker's
   dead_strip) instead of by hollow's whole-text pass over std's code.
4. Debug information stays as decision 0346 has it: every build, edit builds included, embeds
   its DWARF in the executable (crash tracebacks, atos and lldb read it there), std's DWARF
   coming from std's object and joined at the link. No debug map and no separate DWARF for edit
   builds: the oracles below show the join's cost was the linker's, not the DWARF's (see "The
   DWARF join").
5. Landing gates (milestone 0.34.0): a fib edit at most 230 ms and an HTTP server edit at most
   500 ms on darwin/arm64, every cache warm, and no runtime regression on the release-vs-go
   workloads (the risk named under "What it costs"). The fixed point, the goldens and the timing
   suites re-baseline once, in the landing change.

Stages, each measured with `--ab` against its parent before the next: (1) the export data format
and std's object, built once per xo identity, target and flags into the back-end cache; (2) the
program's modules checked against the export data instead of std's sources; (3) SSA and the
whole-program passes over the program's functions only, with std's exported facts; (4) the link
of the program's objects with std's, dead code stripped by symbol.

## What it costs (to decide on)

- Cross-module inlining narrows to the bodies std exports, chosen when std is compiled; a std
  function that today is inlined after the program's specialization may not be.
- hollow's whole-program reach becomes the linker's dead stripping by symbol; data reachable only
  through dead functions, and DWARF for them, need the same treatment.
- Borrow inference, uniqueness (0322), may_fault and unwind pruning stop seeing through std:
  std's facts are computed with std alone and exported. A std function that calls a closure the
  program passes keeps conservative facts; today those are inferred per program.
- Whole-program effect inference and the module check cache's interaction
  (gotchas/module-check-cache-skips-whole-program-passes.md) become per-module contracts.
- Std object builds: a cold build after an xo change compiles std once (about the cold cost of
  today's caches); the cache holds one std object per target and flag set.
- Output identity with today's whole-program build is lost by design: the binaries differ (less
  inlining across the std boundary, different layout). The fixed point, the goldens and the
  timing suites must re-baseline once, and runtime performance must be measured against today's
  (the risk is std calls the program's specialization made cheap).

## Expected effect (an estimate from measured phases, not an oracle)

A fib edit would be the process start (3 ms), the key (1), the check of the program's module
against export data (a few ms: check_module is 4.4 ms of CPU today including std's lazy bodies),
its SSA and passes for two functions (a few ms), its emit and one object's assembly (a few ms),
the link of that object with std's (11.6 ms today, more with dead stripping in place of hollow),
and the exit (9, less with 0382's deferred exit work): about 35 to 45 ms. The HTTP server scales
with its own code, not std's. Measuring this needs a prototype: the first step would be an oracle
that links fib's own part against a std object made from today's parts.

## Oracle measurements (2026-10-27)

Oracle 1, n=8 interleaved, darwin/arm64, quiet machine, at aa91ea517 (a throwaway xo that replays a build's
link inputs through macho_link alone: process start, the link, the exit):

| Row | Median (ms) | How |
|---|---|---|
| xo start and exit (`xo version`) | 9.0 | measured |
| fib's link alone (13 hollowed objects, 2.1M) | 12 (21 with start and exit) | measured |
| the HTTP server's link alone (38 objects, 7.9M), standing in for a full std object | 42.5 (51.5 with start and exit) | measured |
| the program's own check, SSA, passes, emit and assembly for fib's two functions | 5 to 10 | bounded from phase times (check_module 4.4 ms of CPU; assemble 2.2 for 13 objects today) |
| a fib edit today (`xo build`, every cache warm) | 261.5 (mean 261.9) | measured |

A fib edit under this decision: 9 + 42.5 + 5 to 10, about 57 to 62 ms, above the 35 to 45 estimated
above: the link against a full std set costs about 42 ms, not the 12 of today's hollowed objects.
That is still about 4x faster than today's 261 and under Go's 102.

Oracle 2, the same harness and machine, n=8 interleaved, medians with xo's start and exit
(8.0 alone): the HTTP link (38 objects, 7.9M) 51.5 ms; the same without the embedded DWARF
(decision 0346) 21.5; its objects merged into one by `ld -r` (which drops DWARF; the throwaway
linker also dropped the merged object's DWARF-mode unwind entries) 17.0. The system `ld` on the
same inputs, as a reference linker: 36.0, with `-dead_strip` 35.5 (the objects are already
hollowed), merged 32.0.

So about 30 of oracle 1's 42.5 ms is embedding std's DWARF, not linking its code: without it a
full std set links in about 13.5 ms (9 when std is one prelinked object). A fib edit under this
decision would be 8 to 9 + 9 to 13.5 + 5 to 10, about 23 to 33 ms, below the 35 to 45 estimated
above, if the edit build does not relink std's DWARF: std's DWARF kept in std's object and
referenced from a dSYM-style debug map (as ld64's N_OSO entries do), or written once per std
object and copied, are the options to weigh. With today's embedded DWARF the floor stays at 57
to 62 ms.

### The DWARF join

A CPU profile of the HTTP link (40 links in one process, 1000 Hz) put about 13 of its 18 ms of
CPU in the linker's DWARF join (compiler/macho dsym.xo dwarf_data): every DWARF byte copied into
a List[Int], the joined lists copied once per object, and the result written a byte at a time,
with the memory mapping and unmapping that came with them. f59cab86a joins the sections as
Bytes, copied in slices around their patches, with the output the same byte for byte (the
__DWARF sections of llvm-fib and the HTTP server, and their --dsym bundles). Quiet `--ab`, n=10
interleaved, A built at its parent: llvm-fib's edit 245.0 to 242.1 ms mean, its link phase 11.9
to 7.0; the HTTP server's edit 603.5 to 587.8, its link phase 43.7 to 27.1. The HTTP link
without DWARF is about 13.5 ms, so about 14 ms of DWARF remains (writing the larger file is
part of it); the floor of a fib edit under this decision with embedded DWARF is about 9 + 27 +
5 to 10, 41 to 46 ms, and 23 to 33 with no DWARF to join. The rest is the next profile's to
place before stage 4; if it cannot be brought down, std's DWARF prejoined once per std object
(its offsets moved by the program's pieces at the link) is the fallback, still embedded.

## Progress

- Stage 1a-i (d65aa66ad, 2026-10-27): the std tree's modules are declared and resolved before the
  program's, so std's objects, types and effect sets are one block of the checker's tables; no
  output change (identity 200 of 200, `xo check --json` of every errorcheck directory the same).
- Stage 1a-ii (aa7cb9da2 and the opt in XO_STD_EXPORT, 2026-10-27): that block as records with
  no syntax tree, renumbered for another program by one offset per table and by path for modules
  and files, installed in place of the std phases with the trees joined from the parses. A fib
  edit's check (33 ms) was profiled first: std's parses on the prefetch tasks (39 ms of CPU) and
  the main thread's 16.6 ms (declare 4.3, resolve_sigs 4.7). So the records alone buy nothing
  (measured: check 29.9 and 30.7 ms off and on); the gain is stage 3's, when an edit neither parses
  nor lowers std (generic instances in fib's emitted code: 44 of 1,373; the HTTP server's 415 of
  3,391).
- Split (Lead, 2026-10-27): xo-lang-4 builds std's object (1b), its cache kinds and xo dev stdobj,
  and stage 4's dead stripping; Xo Lang 1 stages 1a, 2 and 3 and the facts std exports
  (ssa.std_facts; an emitter section per function from 1b's builder).

## Alternatives

- More per-pass caching (0382 stage 3 and its successors, 0380 stage 3): bounded by the oracle
  runs at about 35 to 50 ms more of fib's 274; reaches the 0.33.0 criterion with 0380 stage 3,
  not Go's range.
- The compile server (0283): keeps the whole program's state in memory between builds, so an
  edit re-runs only what changed without reading caches; complementary, costs resident memory and
  a daemon, and is not a one-shot `xo build`.
- A debug map for edit builds (the objects keep their DWARF, the executable names them, as
  ld64's N_OSO entries do): not needed once the join was fixed, and it would reverse part of
  0346 (a traceback or lldb session would depend on objects in a cache a prune can remove).
- Keep the whole-program build for `--release` and use separate compilation only for edit
  rebuilds (Go has one mode; two modes double what is tested): a variant of this decision to weigh.
