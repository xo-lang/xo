# 0208: arm64 list addressing, loop data pointers, long multiplies

Status: Accepted (2026-10-13)

## Context

Argon2id in Xo retired 2.071x the C's instructions on arm64 and 1.294x
on LLVM (decision 0195, runtime-port-portable-2026-10-13.md), so it
stayed C. On arm64 every List[Int] element access was an address add
and a load, the bounds checks a guard had proved came back in the block
after an `a || b` join, fBlaMka's `2 * lo(a) * lo(b)` was `and`, `and`,
`mul`, `lsl`, and a loop storing into a list that came in shared checked
its count (and reloaded its length) on every store.

## Decision

In both arm64 emitters (internal/arm64 and selfhost/arm64, identical
output) and both SSA builders' rc pass:

1. A list element is addressed `[list, #24 + k * size]` for a constant
   index, `[b, #24 + c * size]` for `i + c` (b the address of element
   i, also used for `xs[i]` when an `xs[i + c]` exists), and `[data, i,
   lsl #3]` (`[data, i]` for Bool) for another one word element, the
   data address computed once per block or, for a list the loop cannot
   move, once per loop as a header parameter (a data plan beside the
   pointer plans of decision 0054).
2. The block after a join keeps the facts about a list's length from
   the block that reaches it (memo lenAt), and an index below a known
   lower bound of the length is not checked.
3. Products of two 32 bit values (masked, `(x << 32) >> 32`, or in
   range, one masked or extended) are umull or smull, a wrapping add of
   one used once umaddl or smaddl, and a wrapping add of a wrapping
   shift by a constant an add with a shifted operand.
4. Load and store pairs form across labels nothing refers to (the
   function's text and its call site table are searched).
5. A List parameter of a loop header that an update takes directly on
   every iteration, passed back updated or unchanged on every back edge,
   not retained, is made unique (reuse) on the edges into the loop, when
   each leaves a block that only jumps.

## Consequences

Argon2id in Xo: arm64 2.212x to 1.125x the C, LLVM 1.294x to 1.254x
(bench/results/arm64-codegen-2026-10-13.md); still above decision 0180's
bar, so it stays C; most of the arm64 rest is filling 4 MiB with zeros
by pushes. A loop of item 5 that runs no iteration copies a shared list
it did not need to. Generated code changes wherever lists are indexed
(internal/arm64 goldens, internal/ssa rc goldens).

## Alternatives

- Reuse at the first update, as before: three instructions and a length
  reload per store for the whole loop.
- Peel the first iteration of such loops instead of item 5: no wasted
  copy for a loop that runs no iteration, but code size per loop.
- Make interprocedural length facts (a callee that only sets elements
  keeps the length): left for later; a source guard does it now.
