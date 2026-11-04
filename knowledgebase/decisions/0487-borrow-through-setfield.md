# 0487: a borrowed projection outlives a write to another field

Status: Accepted (Xo Lang 1, 2026-11-03; the Lead's Option-of-a-view item,
an extension of decision 0186 within its rules)

## Context

Decision 0186 leaves a projection (a field, a `List.get` payload, a loop
element) uncounted when nothing consumes its owner's root while the
projection is alive. A `var` struct written in place lowers each
`b.f = x` to `w = setfield F, b, x`, which consumes the root `b`. So a
write to any field of b, even an Int unrelated to the projection, kept a
projection of another field counted: a retain and a release (an out of
line drop call) per use. ByteBuffer.read_until's fast path (`if let
Some(s) = b.segs.get(b.head) { ... b.pos = ...; b.scan_to = ...;
Some(view of s) }`) paid them per record, about 0.7 ns of a 14 ns record
(the Lead, 2026-11-03; bench/perf/bytebuf's buf against views, twin 1.049).
The same shape is common in std: a cipher state's `k.seq += 1` while its
Gcm is borrowed (tlsrec.seal_at), a parser's position written while a
segment is read.

## Decision

- compiler/ssa/borrow.xo `decide_borrow` follows the owner's root through
  setfields (`carries`): a setfield of the root's whole value whose field
  does not overlap the owner's path holds the owner's leaf at the same
  path from its definition on. The projection stays borrowed when every
  other consume of a root of that chain falls outside its life, and each
  setfield of the chain dominates every later use of the projection it
  reaches (`chain_dominates`, verify.xo's dominators); a loop that writes
  the field before the projection's next use keeps it counted.
- A projection of a borrowed projection inherits its parent's chain (one
  made after the setfield names the setfield's value as its owner).
- compiler/ssa/rc.xo `owner_use` names, at each use, the owner's leaf in
  the last chain value before it (`bchain`, `bpos`, `bidom`).
- The rc verifier (verify.xo `alive`) follows a parent leaf moved into a
  setfield of another field (`carried_alive`), through every setfield of
  the chain (a second write consumes the first's value): it models
  borrowed projections on its own
  (gotchas/rc-verifier-models-borrows-separately.md).
- A consume of the projection itself checks the owner's later uses
  through the chain's setfields too (`used_after` of each chain value).
- An edge argument of no heap type is not a consume of its root (an Int
  field read of the written struct passed to a join, `x = if c { b.f }
  else { .. }`, moves nothing; it ended every borrow of b's projections).
  This applies to every borrow decision, not only chains.
- std: ByteBuffer.read_until writes scan_to on both paths, not in an `if`
  whose join passed the ByteBuffer through a block parameter (a heap
  argument, which still ends the chain).

## Consequences

- ByteBuffer.read_until's fast path (the real binary, otool; post-rc
  dump): one retain a record (the returned view), from two retains and
  two releases. `xo asm` showed no change at first: it lays code out
  unlike `xo build` (gotchas/xo-asm-not-the-built-binary.md).
- bench/perf/bytebuf small_buf (`--ab`, linux/arm64 host 1, 2026-11-04, n=3):
  B/A 0.940 (0.934 to 0.976), 13.30 to 12.51 ms for 1,006,704 records,
  about 0.8 ns a record; max RSS unchanged. The first version (one
  setfield, no join or Int edge fix) measured 1.001 (noise).
- The first version passed its gate while a two-write chain inlined into
  a caller failed the inline verifier (gotchas/gate-passes-on-inline-fallback.md);
  golden borrow_setfield_join guards it.
- Tests: goldens borrow_setfield_chain (a projection, a projection of it
  made after the write, a loop, an early return) and borrow_setfield_join
  (two writes inlined into a caller; an Int join after a write); a codegen pattern in
  test/codegen/retains (seg_head_len: no call at all, which fails on the
  compiler before this change).
- Gate (linux/arm64, 2026-11-03): every arm64 golden under XO_FREE_CHECK=1
  (538 pass), codegen patterns, repotest, the LLVM sample with Go parity,
  the fixed point.

## Alternatives

- Reorder std's read_until so its field writes follow the last use of the
  segment: fixes one function, not the class.
- Scalar replacement of `var` struct parameters (each field its own SSA
  value): a larger change touching every pass that sees the struct.
- An Option of a view returned in registers: the retain the Lead measured
  was the List.get payload's, not the Option's; returning it in registers
  does not remove the count this change removes.
