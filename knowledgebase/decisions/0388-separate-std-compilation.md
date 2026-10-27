# 0388: Std compiled once to objects and export data; an edit compiles only the program's modules

Status: Proposed (Xo Lang 1, 2026-10-27, for the owner; the 0.33.0 edit-rebuild criterion's
structural option, decision 0382's measurements)

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

## Decision (proposed)

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

## Alternatives

- More per-pass caching (0382 stage 3 and its successors, 0380 stage 3): bounded by the oracle
  runs at about 35 to 50 ms more of fib's 274; reaches the 0.33.0 criterion with 0380 stage 3,
  not Go's range.
- The compile server (0283): keeps the whole program's state in memory between builds, so an
  edit re-runs only what changed without reading caches; complementary, costs resident memory and
  a daemon, and is not a one-shot `xo build`.
- Keep the whole-program build for `--release` and use separate compilation only for edit
  rebuilds (Go has one mode; two modes double what is tested): a variant of this decision to weigh.
