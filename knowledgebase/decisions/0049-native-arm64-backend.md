# 0049: Native arm64 backend from the SSA IR

Status: Accepted (2026-10-08); extended by 0050 (closures, boxes,
equality, wide results). Implemented in `internal/arm64`, wired as
`xo build --backend=arm64` (`internal/build/arm64.go`). Evidence:
`bench/results/arm64-native-2026-10-08.md`.

## Context
Decision 0040 lists a direct arm64 backend as one of the native paths,
next to LLVM. The LLVM backend (decision 0046) depends on clang with LTO
for every build and gives no control over code shape. Decision 0045 made
`internal/ssa` (after `InsertRC`) the input every native backend lowers
from, so a second backend costs only the lowering, the register
allocator, and the emitter. The development host is darwin/arm64 (Apple
M4 Pro).

## Decision
Add `internal/arm64`, which lowers a module after `InsertRC` straight to
AArch64 assembly text, assembled and linked by the system C compiler
(`cc`, or `$XO_CC`) together with the unchanged C runtime
`std/llrt/xo_rt.c`. No LLVM, no LTO.

- **Values are words.** Every SSA value is flattened into 64 bit words:
  Int, Bool, List one integer word; Float one floating point word; Str
  two (object, length); structs and tuples the words of their fields;
  enums a tag word plus the most payload words of any variant, one word
  per field word (Float fields as bits). Units and capabilities have no
  words. In memory (list elements) a word is 8 bytes, except a `List[Bool]`
  element, which is one byte. This layout is the backend's own; it does
  not share memory with LLVM code, and the runtime only sees element
  sizes and callbacks.
- **MIR.** Instruction selection produces instructions over virtual
  registers with assembly templates (`add $d0, $u0, #1`). Struct and tuple
  construction, field reads, `setfield`, and payload reads of integer
  words are register aliases (SSA values are immutable), so they emit
  nothing. Block parameters become parallel moves at the end of each
  predecessor; a conditional edge with arguments gets a block of its own.
  Calls, entry, and return are pseudo instructions whose ABI moves are
  resolved after allocation.
- **Register allocation.** Liveness by data flow; precise live ranges
  with holes per virtual register (positions: uses at 4i, call clobbers
  at 4i+1, definitions at 4i+2); move coalescing (registers joined by a
  parallel move whose ranges do not overlap share a location, most
  frequent moves first); then greedy first fit in order of spill weight
  (uses weighted by 8^loop depth, per range length) against the ranges
  already placed in each physical register, preferring ABI registers
  and coalescing partners. Calls occupy every caller saved register for
  one position, so values live across a call land in x19 to x28 or d8 to
  d15. A register that fits nowhere is spilled for its whole life to its
  own 8 byte slot and reloaded into a scratch register at each use. This
  is linear scan's model (one location per value) without its sweep:
  checking range sets per register handles the holes linear scan needs
  extra machinery for. Scratch: x15, x16, x17, d29 to d31; x18 is never
  touched (Apple reserves it); x29 frame pointer, x30 link register.
- **Calling convention.** AAPCS64 for every call: integer words in x0 to
  x7, floating point words in d0 to d7, results the same; further Xo to
  Xo arguments in 8 byte stack slots (more than 8 result words of a class
  is rejected). Runtime calls take at most four scalars and none is
  variadic, so the Apple variadic rule never applies.
- **Frames.** A function that calls, saves, or spills keeps a frame
  record (x29 points at it; one `stp x29, x30, [sp, #-size]!` when no
  call passes stack arguments). Callee saved registers are stored in
  pairs with the higher register first (`stp x20, x19`), as LLVM does
  (the other order cost 1.4x on recursive fib,
  `gotchas/arm64-stp-pair-order.md`). A leaf function with nothing to
  save has no frame, like clang's default on Darwin (frame pointers in
  non leaf functions); x29 still points at its caller's record. sp stays
  16 byte aligned.
- **Faults.** A checked operation branches to an out of line stub that
  moves the operands into argument registers and calls the runtime's
  fault function (same messages and exit status 70 as the other
  backends). A checked add, subtract, or negate may give its result the
  operand's register (so a loop counter updates in place); its stub
  recomputes the operand by undoing the wrapped operation.
- **Reference counting.** Retain of a Str or List is inline; release
  calls a generated helper (decrement, or free at one through
  `xo_rc_free` / `xo_rl_free`); enums, structs, and tuples get generated
  dup and drop helpers that take their words in x0 to x7, and list
  element callbacks for `xo_rl_copy` and `xo_rl_free`. `reuse` is an
  inline count check with a cold copy; list updates are inline.
- **Local optimizations** (all inside `internal/arm64`, none in
  `internal/ssa`): per block memos of constants, list lengths, bounds
  checks, and element addresses, inherited from the dominator; a list's
  length travels with it through block parameters as an extra register
  (`gotchas/arm64-loop-carried-store-forwarding.md`); dead code removal;
  stores of `list.set` that write back an unchanged word are dropped;
  bounds from dominating branches remove overflow checks of add and
  subtract by a constant (`n - 1` under `n >= 2`); division and remainder
  by powers of two (a plain shift when a branch proved the low bits
  zero); multiplication by 2^k and 2^k + 1 with shifts;
  `x % 2^k == 0` as a bit test (`tbz`/`tbnz` for bit 0); constants
  hoisted out of loops that make no call on their hot path; small leaf functions
  (at most 40 instructions, no calls of Xo functions) are inlined on the
  SSA, after which the module is verified again (reference counting
  balance included) and generation falls back to no inlining if it does
  not verify; jumps are threaded through blocks that test a constant tag
  or the Bool an inlined predicate returns; loop headers that only test
  are duplicated at the back edge.
- **Targets.** `--backend=arm64` builds for darwin/arm64 (Mach-O,
  underscore prefix, `@PAGE` relocations) and linux/arm64 (ELF, no
  prefix, `:lo12:` relocations). The host is assembled and linked; another
  arm64 target is emitted as assembly only (`build.Result.Asm`). Programs
  outside the SSA subset are an error (there is no AST fallback as in
  the LLVM backend).

## Consequences
- All seven `bench/perf/llvm/xo` programs and the 16 SSA path golden
  programs of `internal/llvm` run natively with output identical to the
  other backends and no allocation live at exit. On the bench programs
  the native code runs at 0.93x to 1.25x of the LLVM backend's time
  (faster on nbody and collatz, even on fib) with the same peak memory. Builds take
  the same time end to end (about 110 ms for a bench program) because
  both backends compile `xo_rt.c` with clang on every build; caching the
  runtime object would make arm64 builds a few milliseconds plus the
  link.
- The code generator is about 3,900 lines that the project controls:
  code shape decisions (frames, fault stubs, element layout) are local
  and testable with golden assembly (`internal/arm64/testdata/*.s`).
- What LLVM does that this backend does not: loop invariant code motion
  of loads (beyond list lengths), induction variable strength reduction,
  scheduling, jump tables for `match`, inlining across larger functions,
  rematerialization of spilled constants. These are the remaining gaps
  on sort and interp.
- linux/arm64 is not run in CI; the tests assemble its output as ELF with
  clang when available.
  Update 2026-10-09: linked and run in a linux/arm64 container
  (`scripts/linux-test.sh arm64`); every golden passes
  (`bench/results/linux-2026-10-09.md`). Still not in CI.
- Concurrency (tasks) will need the same atomic or per task counts as the
  LLVM backend.

## Alternatives
- **LLVM only** (decision 0046): best optimizer, but every build needs
  clang and LTO, and code shape is not ours to control.
- **Go's assembler or Go's SSA backend**: tied to Go's ABI, runtime, and
  garbage collector; Xo's reference counted memory and fault paths do
  not fit.
- **Cranelift or another code generator library**: not Go, a large
  dependency against the standard library preference.
- **Graph coloring allocation** (Chaitin, Briggs): better on high
  pressure code, more code; the bench programs never spill with first
  fit over range sets, so it is not needed yet.
- **Classic linear scan** (Poletto and Sarkar, one interval per
  register): simpler sweep, but intervals without holes make loop
  counters and their increments conflict, which costs a move per
  iteration; range sets avoid that.
- **Spill slots shared by interval coloring**: smaller frames; not needed
  while nothing in the bench programs spills.
