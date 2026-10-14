# Static words taken twice, and a module's own atomic order constants

Status: Observed (2026-10-14)

## Symptom

Two runtime modules corrupt each other's process state with no build
error, or a lock built on the slot atomics lets two threads in.
On 2026-10-14 four branches claimed static words in 14 to 23 within a
few hours (fault.xo, tzhost.xo, ctx.xo, heapos.xo): heapos.xo's lock
word was ctx_root and its chunk counter the pool record once B17 merged.
Separately, heapos.xo defined `ACQUIRE = 2` (release in rt.xo's
numbering), so its central lists' lock CAS took no acquire and, on
arm64, two threads were handed one magazine (a free list whose head was
a reference count; xo segfaulted building itself). tzhost.xo had
`RELEASE = 3` (acq_rel: stronger than needed, not unsafe).

## Cause

`rt.static_word(k)` is a plain index into one 64 word table, claimed
only by a prose list in rt.xo; and the slot atomics take their order as
an Int that each module redeclares as its own constants.

## Fix

internal/repotest (part of `go test -short`):

- TestStaticWords: rt.xo's table lists one range per module, one line
  each (`///   LO-HI std/native/llrt/name.xo: what`); every
  `rt.static_word(K)` of a module must fall in its range and no two
  ranges overlap. Claim a range in rt.xo in the change that first uses
  it, after checking current main.
- TestAtomicOrderConstants: every `const RELAXED/ACQUIRE/RELEASE/
  ACQ_REL/SEQ_CST` under std/ equals rt.xo's 0, 1, 2, 3, 4.

How the heapos.xo case was found (B21, bench/results/heap-platform-2026-10-14.md):
the crash printed nothing and passed with one emit job; gdb stopped in
alloc.alloc on a free list head of 1, and a bisection sending each part
back to the C named the lock, though B17's words 15 and 16 (then also
heapos.xo's) muddled it until the ranges were moved. The same race made
s1 about five times slower before it crashed.
