# 0224: Shrink wrapping and tail calls in the arm64 emitter

Status: Accepted, implemented (2026-10-14, selfhost/arm64/shrink.xo;
measurements in bench/results/shrink-wrapping-2026-10-14.md).

## Context

The arm64 emitter made a function a leaf only when it had no call at
all, and a function with a frame made it at its entry: the frame record
and every callee saved register it used were saved and restored on
every path (gotchas/arm64-call-anywhere-costs-frame.md). Runtime code
in Xo (decisions 0180, 0195, 0207) is full of functions whose common
path calls nothing and whose rare path does (a free list pop that
refills, a list release that runs drops), and of functions whose last
act is a call (an export forwarding to the function it names, a free
that hands a large block to the C). B13 (zero-C audit, the list objects
in Xo) missed decision 0180's bar on arm64 only (alloc small 1.266 of
the C's instructions on linux/arm64, while LLVM, whose clang shrink
wraps and tail calls, was near 1.0) and named this as the cause.

## Decision

1. Tail calls. A call whose results are the function's, returned by
   nothing but parallel moves and jumps after it, with no stack
   arguments or results, no landing pad, not a system call or a variadic
   C call, in a function whose frame holds no memory a callee may get
   the address of (byref.xo's memory, the $OUT slot), is emitted as its
   argument moves, the epilogue without `ret`, and `b sym` (`br` for a
   closure or C pointer call). A function whose only calls are tail
   calls is a leaf.
2. Shrink wrapping. Before register allocation, the blocks that need
   the frame by their instructions alone (calls but tail calls, $OUT,
   the frame's memory) are widened to the region of blocks both reached
   from one and reaching one (a loop around a call is inside). When the
   function can return outside the region, has no landing pads and no
   stack arguments or results, and the region has at most six edges in
   and out, each edge in gets a save block (the prologue) and each edge
   out a restore block (the epilogue without `ret`; a following block
   that only returns is copied after it). The registers named or live
   on both sides get new names inside the region, and the edge blocks
   copy those live across them (never coalesced), so a value live across
   the region's calls takes a callee saved register only inside it.
3. After allocation the plan is kept only when no block outside the
   region uses a callee saved register or a slot; otherwise the frame
   is made at the entry as before, and the edge blocks stay plain
   copies.
4. Unwinding: the blocks without the frame are laid out first and the
   region after them, so the call frame rules change once, at the
   region's first block, with only `.cfi_def_cfa` and `.cfi_offset`
   (all selfhost/macho assembles). A fault stub of a block without the
   frame makes the whole frame before its call, so every stub unwinds by
   the frame's rules.

## Consequences

- fib returns its small cases with no frame; the runtime's alloc.alloc
  and alloc.free and the C ABI forwarders have no frame; the self build
  of xo runs 1.5 percent fewer instructions (0.985); bench/perf does
  not regress (at most 1.004); B13's list objects move from 1.242 to
  1.201 (alloc small) and 1.086 to 1.058 (text split) on the Mac.
- A tail call drops the caller's frame from a backtrace taken inside the
  callee; faults unwind correctly (the callee returns to the caller's
  caller, and a call with a landing pad is never a tail call).
- Darwin's compact unwind encoding of a shrink wrapped function
  describes the frame for its whole range: asynchronous unwinding (a
  profiler's sample) in a block without the frame reads the frame's
  slots. Synchronous unwinding (faults, landing pads) only starts at a
  call, which is inside the region or in a stub that made the frame. On
  ELF the DWARF rules are exact but for the prologue and epilogue
  instructions, as before.
- Code grows by the edge blocks (the self built xo 51,864,624 bytes with
  only single entry shrink wrapping, 51,947,936 with the regions).
- internal/arm64 TestGoldenAsm's goldens changed with this decision.

## Alternatives

- One entry block only (the first version: the frame made at the head
  of the one block entering the framed blocks, which had to be closed
  under successors): it found few functions, since most paths that need
  the frame rejoin the frameless path before returning (B13's alloc
  small 1.242 to 1.217 only).
- LLVM's save and restore points (one dominating save, one post
  dominating restore): simpler in the unwind rules but misses a release
  loop whose exit rejoins a path without the frame (obj_rl_free).
- A frame record without the saved registers on some paths (two levels
  of frame): not needed by the cases measured.
