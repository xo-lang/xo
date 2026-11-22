# 0742: The program's link sees std's bitcode with what the program reaches

Status: Proposed (2026-11-22, task #607). Not built. The recommendation
below is to not build it now.

## Context

Decision 0738's opt in (`XO_SEPCOMP_LLVM=1`) compiles std once for the
LLVM backend and links a program against std's objects; clang's time on
a program went from 4.1 to 30.8 s to 0.08 to 0.64 s. After e43aa14699 it
runs 1.029 of the whole program's time over 71 workloads on host 1 (from
1.486 at stage 2). Nine rows remain at 1.10 or more: digest sha256_small
1.180, fault defer 1.177, text cut 1.167, portable str_hash 1.145, fault
isolate 1.145, rsa sign2048 1.135, text replace 1.132, fault owned 1.131,
json maps 1.102.

What those rows lack was found by disassembly (0738, "Two causes found
by disassembly" and after): in a whole program a function that only one
live function calls is `internal`, and LLVM inlines it whatever its
size. The whole program's cabi.fault_raise_cur holds five functions of
the unwinder, 5,632 bytes; std's objects have each as a function of its
own. Which functions have one caller is a fact about a program, after
its unused code is dropped: across all of std they have several. Marking
the calls that are alone across all of std was measured (0738: text
+15%, the unwinder unchanged, no row clearly faster).

So the remaining gap can only be closed by a step that knows what the
program reaches. That step is this decision.

## Decision (proposed)

At the link of a program built against std's LLVM objects, std takes
part as bitcode, not as objects:

1. The thin link (lld with std.N.bc, kept beside the objects since
   stage 3, and the program's module) computes what is live from the
   program's entry, and makes each of std's symbols that no other module
   reaches local to its module.
2. Each of std's modules is compiled again for that program, the dead
   functions gone and the local ones inlined as in a whole program. A
   cache keyed by the module and by what the link decided for it
   (lld's `--thinlto-cache-dir`) returns the object when another
   program decided the same.

## Cost

Unmeasured in this form; what is measured says it is high:

- Every program compiles the part of std it reaches again. That is the
  whole program's clang time less the part LLVM never sees: the 4.1 to
  30.8 s the opt in removed come back in proportion to what the program
  reaches. The cache helps only between programs that reach the same
  functions of a module, which two different programs rarely do.
- 0738's one link over all of std's bitcode took 100 to 124 s (orc4.out)
  and held 15 to 19 MB of code, since nothing told lld what to drop.
  With the entry as the only root it would be less; how much less is
  the first thing to measure.
- A third kind of std artifact to keep in step (objects, bitcode for
  imports, bitcode for the link), and lld becomes required for the
  opt in on every target, which Apple's clang does not ship.

## What it should close

The nine rows above, to about 1.00: the same passes would run over the
same live code as in a whole program. It adds nothing to the 62 rows
already within 10%.

## Recommendation

Do not build it now, and take the opt in as it is for what it is for:

- The opt in is a build time tool for goldens and edit loops.
  `--release` builds the whole program (compiler/xo sepenv.xo), so no
  shipped binary runs the 1.029, and `xo bench` refuses the opt in.
- The step that closes the gap is, in cost, most of the whole program's
  build, which the opt in exists to avoid.
- 1.029 over 71 workloads with the worst row at 1.18 is inside what a
  test or an edit loop tolerates.

If the owner wants the rows closed all the same, the cheaper variant to
try first is a fixed kernel: the few modules the nine rows lose in
(the unwinder, llrt's object and allocator code, prelude's string
functions) take part in the link as bitcode and the rest of std stays
objects. Its cost is bounded by the kernel's size, and a stage 0 would
measure it by hand before any code: link time, the nine rows, the size.

## Consequences

- If accepted as recommended: 0738's remaining work is the default (on
  ELF), darwin's plain objects, the size, and `xo test`; the nine rows
  are recorded as the opt in's known cost.
- If built: the opt in's build time advantage shrinks by the reached
  part of std, to be measured at stage 0 before anything else.

## Alternatives

- Mark, in std's own build, the one call of each function nothing else
  in std names: measured, no gain (0738).
- Raise ThinLTO's import limit: measured, no row moved (0738).
- One link over all of std's bitcode without roots: measured, 100 to
  124 s a link and no better on average (0738).
- A profile of each benchmark fed back into std's build: closes rows for
  the programs profiled only.
