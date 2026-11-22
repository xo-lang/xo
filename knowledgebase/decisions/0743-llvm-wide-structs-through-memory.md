# 0743: The LLVM emitter carries wide struct values through memory

Status: Proposed (2026-11-22, task #620). Not built (lead, 2026-11-22:
a design stub for later).

## Context

The LLVM emitter makes every struct value a first class LLVM value: a
literal is a chain of insertvalue, a field read an extractvalue, a copy
a load and a store of the whole type, a parameter and a result the
struct by value. For a struct of a few fields LLVM turns that into
registers. For a very wide one it does not scale.

internal/llvm's big_struct golden (a struct of 600 fields, 900 words)
takes clang 37.2 s at -O2 and 2.6 s at -O0 (host 1 alone, clang 23.1.3,
three runs within 1%; ~/xo-lead/notes/xo-lang-1/p620/h1t.out). Its main
reads one value, the result of one call, field by field 11,701 times,
and LLVM keeps the 900 words live between the reads: main takes 9.2 s
of the optimizer's 18.6 s (InstCombine 6.0 s) and most of the code
generator's 17.9 s.

Measured and ruled out (p620/gr1.out to gr4.out, on a branch):

- One call of the struct's own helper in place of a release of each
  field: the inlined releases fell from 12,703 to 1,603 and the text by
  10%; clang's time did not move.
- A frame slot for a wide value read many times, stored once, its field
  reads loads from the slot: 39 to 55 s. LLVM splits the slot into its
  fields and promotes them again.

- Compiling the heavy function optnone, as a large constant
  initializer is (a budget of 2,000 field operations on structs of 64
  fields or more, which takes big_struct's main alone; branch
  l620-optnone-tried, with its test). By hand on the IR it took the
  module from 37.2 to 22.9 s; the real build's clang phase went from
  44.7 to 37.5 s (three runs each, host 1 alone, h2t.out), 16%, since
  the module wide passes and the helpers of Map[Str, Big] still see
  the 900 word type. It cost 35% more text (1,585,587 to 2,139,059
  bytes), 33% more resident memory (2,352 to 3,120 KB) and about 7% of
  the program's run time (`xo bench --ab`, 10 rounds: 1.075, too short
  for a verdict). Not landed: a rule in default code generation for a
  sixth of one test program's build.

Nothing is in place: big_struct builds as before.

## Decision (proposed)

A struct above a size bound never exists as a first class value in the
LLVM emitter:

1. A value of such a type is a pointer to memory that holds it: a slot
   of the frame for a temporary, the storage itself for a value read
   from a list, a field or a box.
2. A function returns it through a pointer the caller passes (sret) and
   takes it by pointer, the callee not writing through it.
3. A copy is a memcpy; a field read is a load at its offset, a field
   write a store into a copy or, when the old value is dead, in place;
   a join of two paths is a join of pointers.
4. Types that hold such a struct by value (an Option or a Result of it,
   a tuple) are wide with it.
5. The slots are not promotable: each is written by memcpy or passed by
   address, so LLVM does not split them into fields.

## Consequences

- Every place the emitter moves a value learns the second form: calls
  and returns, block parameters, stores into lists, boxes, closures and
  enum payloads, the reference counting helpers (which have a by
  pointer form already), debug information.
- The C ABI exports and the runtime's calls keep their types; a wide
  struct does not cross them by value today.
- Frames grow by a struct's size for each temporary alive at once;
  lifetime markers let LLVM share the slots.
- The arm64 and x86 backends are not touched: they carry structs in
  memory already.
- big_struct's build (44.7 s in clang, most of an LLVM golden run's
  time on one program) is what it would shorten; no other program in
  the tree has a function near the budget above (210 bench/perf
  programs read, ob1.out).

## Alternatives

- The optnone budget alone: measured (above), 16% for a function that
  then runs unoptimized.
- Lowering the program's own optimization level when it holds a wide
  struct: every function pays.
- A frame slot for wide values read many times: measured, slower.
