# 0403: Profile-guided code layout

Status: Proposed (draft 2026-10-29, xo-lang-6; the owner's 0.35 item 9).
Extends 0075 (no alignment by default, the pad knob), 0238 (the sampling
profiler), 0388 (separate compilation, the std store), 0398 (x86 jump
padding).

## Context

Where code lands decides a measurable share of its speed with the code
unchanged: interp ran 204.5M to 281.4M cycles over 16 placements of one
instruction stream (bench/results/interp-placement-2026-10-28.md, the
dispatch `br` landing well or badly for the indirect predictor), collatz's
cycles moved 2.2 percent and nbody's 1.1 over 16 shifts (0075). Blanket
alignment was tried in 0075: loop tops at 64 bytes cut collatz's spread to
0.7 and nbody's to 0.2 percent but made the median 1 percent worse, since
every loop top paid the padding; it stays off. Go's PGO and LLVM's BOLT
spend the alignment and the packing only where a profile says the code is
hot, which is the missing piece here: the backend aligns nothing, orders
functions as the module declares them (gen.xo exports: g.m.funcs in order),
and the store link places std's atoms in the store's order.

## Decision

1. The profile is the sampling profiler's (0238, `XO_PROF=cpu`, the .xoraw
   file), reduced by `xo prof --layout OUT` to a text file, `xo.layout`,
   that the user keeps beside the main package (Go's default.pgo): one line
   per function, `fn NAME WEIGHT` (the mangled symbol name, the sample
   count), and per sampled line `line NAME LINE WEIGHT`. Keyed by name
   and source line, so it survives edits: a function that disappeared is
   ignored, one that changed keeps its weight (as Go's PGO treats a stale
   profile), and a line that moved loses its block weight but not its
   function's.

2. What the layout does, in two stages of code:
   - Stage 1, placement at the link. Functions are ordered by the profile:
     hot functions first and contiguous (by weight, with a caller placed
     before its hottest callee when both are hot: a simplified Pettis and
     Hansen ordering over the call graph the SSA already has), cold ones
     after, the unprofiled in their declared order. This is one ordering
     applied in both link paths: the whole-program emission order in
     gen.xo, and the atom order of link_store for a separately compiled
     program, so std's hot atoms (the store is read atom by atom) move
     next to the program's hot code without rebuilding std.
   - Stage 2, inside hot functions. Blocks with no samples move to the
     function's end (hot and cold split, the MIR block order), and the
     head of a hot loop and a hot indirect-branch (`br` table) block are
     aligned to 64 bytes with padding that is never run (0075's finding
     applied where it pays). These change the function's code, so they
     enter the function's cache key (the profile's hash, as a codegen
     input listed in internal/xotool codegen_test.go).

3. Reproducible builds: `xo.layout` is a source of the build: its hash
   is in the build info's sources (binfo_sources), in the function and
   object cache keys (stage 2 only), and in nothing else; the same sources
   and layout file give the same bytes twice and with the cache on or off
   (TestLayoutIdentity; a whole and a separate build differ by their link
   paths, with or without a layout).

4. Separate compilation and the std store: stage 1 needs no std change
   (ordering is the link's). Stage 2 applies to the program's own functions
   and the std instances compiled into the program; std's precompiled
   bodies keep their blocks. A per-profile std variant (the store rebuilt
   with the profile, keyed by its hash, about 10 s once) is a later opt-in
   if the measurements show hot std bodies that need splitting.

5. Default: `xo build` and `xo run` use the module root's `xo.layout`
   when it exists (one profile per module; a module with several main
   packages names one with the flag); `--layout=FILE` names another,
   `--layout=off` ignores it. Nothing changes for a program without one.
   The profile names the target it was taken on (`target OS/ARCH` on its
   first line, from the .xoraw's build info): a build for another target,
   or with --race, or by the LLVM or Go backend, ignores the file with one
   line on standard error (`layout: xo.layout is darwin/arm64's, ignored
   for linux/amd64`); stage 1's ordering could apply across targets, but a
   profile taken elsewhere says little about this target's placement, so
   nothing is applied rather than half.

6. Measured by: interp, collatz and strings at the 16 `XO_ARM64_PAD`
   placements, quiet `xo bench --ab` against the same binary without the
   layout. The goal is the spread (max over min cycles across placements)
   shrinking to the noise, with the median no worse; a result that only
   moves the median at one placement is the thing 0075 warned about.
   Stage 2's alignment pays bytes at every hot loop head and `br` block, so
   its results carry a code-size row (the program's and xo's own `__text`,
   as every codegen change does) beside the spread and the median.

## Consequences

- A new input to the build (`xo.layout`), a new `xo prof` output, two
  backend passes (block order, hot alignment) and a link-time ordering
  in both linkers. The Go backend and wasm ignore the layout.
- Measurements of code shape changes gain a fixed layout: with a profile
  the pad sweep should flatten, so a change can be judged at one placement.
- The profile is advisory: a wrong or stale one costs speed, never
  correctness; the tests build with and without it and compare outputs.

## Alternatives

- Blanket alignment (0075): costs every loop top, rejected.
- BOLT-style rewriting of the linked binary: needs a disassembler and
  relocation recovery for every target; the compiler has the blocks.
- An instrumented build (counters per block): exact counts, but a second
  build kind and a run under instrumentation; the sampling profiler is
  already there and its line attribution is enough for blocks.

## Stage 1 built (2026-10-29, xo-lang-6)

`xo prof --layout` (compiler/xo prof.xo prof_layout, layout_lines),
the layout file read and ordered (layout.xo parse_layout, layout_order),
the driver (main.xo: the module root's xo.layout or --layout, the hash
among the build info's sources, a foreign target ignored), the order in
the parts (compiler/arm64 parallel.xo fn_order_of, used by part_lists
and gen.xo). Tests: layout_lines, parse_layout and layout_order in
prof_test.xo; internal/arm64 TestLayoutIdentity. The first measurement
(the alignment experiment and the pad sweeps) follows in bench/results.

Stage 1b (the same day): the store link's text atoms in the layout's
order (compiler/macho link.xo LinkOpts.order and place(); the driver
mangles the file's names with the emitter's rule, arm64 mangle_fn, and
hands them through Ctx.layout_order); darwin only, the ELF placer
computes offsets as it collects chunks and is left for the linux
measurement.

## Stage 2 measured (2026-10-29): held

bench/results/pgo-layout-stage2-2026-10-29.md. The one-binary noise
floor (one binary run in 16 slots x 7 rounds) equals the sweeps' spread
for interp (1.139 against 1.070 to 1.136) and collatz (1.008 against
1.009): their placement spread is gone since threaded dispatch (0397),
and what remains is a 50 ms run's noise; strings keeps about 4 percent.
The hot blocks' alignment removes nothing and costs 2 to 4 percent at
the median; the cold split flattens the spread but costs interp 14 and
strings 13 percent, since a 500 sample profile leaves hot-path blocks
unsampled and they count as cold. Both stay out of main (the code is in
xo-lang-6's worktree notes); on main: stage 1 (the function order) and
1b (the store link's order). Next, if wanted: a split that keeps a block
hot when it lies between sampled blocks in the CFG, measured on a run
long enough that the floor is under a percent, and the measurement rule
(agent-tools.md "Measuring": the one-binary control beside every sweep).

## Stage 2b measured (2026-10-29): held

bench/results/pgo-layout-stage2b-2026-10-29.md. The split that keeps a
block hot when it has a sampled line or dominates or post-dominates a
sampled block (so a loop's header, a sampled block's only successor and
the merge after a sampled branch stay hot), measured on 370 to 740 ms
runs of collatz, strings and interp with quiet `xo bench --ab` (15
rounds, the one-binary floor beside every pair, placements 0 and 8):
every pair is inside its floor (collatz 0.995 and 1.004, strings 1.006
and 0.979 beside a placement pair of 1.027, interp 0.997 and 1.004), and
the split moves almost nothing: a 10 kHz profile samples every block of
a hot loop, so collatz and interp keep every block in place and strings
moves a few error-path blocks. Held like stage 2: the code stays out of
main (xo-lang-6's notes, 0403-stage2b-split.diff); on main stage 1 and
1b. A block level stage would need a program with real cold paths inside
hot functions; none of the three has one.
