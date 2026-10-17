# 0255: Preserve-most drop helpers on arm64

Status: Rejected (2026-10-17, xo-4, with the lead): measured slower; the
experiment's code stays on branch droppm, not on main.

## Context

On arm64 a release of a Str, List, or Handle value is a call of a per
type drop helper (5 instructions on its fast path: load the count,
compare, decrement, store, return). In bench/perf/tls's client these
are 32,326 of 47,146 calls (69%), and x509.parse's 2,721 of 2,874
(95%). Every call clobbers the caller saved registers, so the idea was
that values live across them spill: 13% of the client's static
instructions are spill traffic, all in functions that save all ten of
x19 to x28.

## Decision

Keep the drop helpers as plain calls. Three conventions were built and
measured against main with the scratch reload pass (lsr.xo
reload_reuse) applied to both sides:

- A: a preserve-most helper per type with an entry per register the
  object may be in (x0 to x17, x19 to x28), so the site is one bl; the
  fast path uses x17 only; the last reference saves x0 to x15 and the
  frame record and calls the plain helper. The allocator sees an
  instruction that clobbers x16, x17, x30, and the floating point caller
  saved registers.
- B: as A, keeping only x0 to x7 (x8 to x15 clobbered).
- C: the fast path inline at the site (ldr, subs, b.gt, str), the last
  reference through A's slow entry.

## Consequences

| | TLS client text | hello | HTTP server | p256 / x25519 / tls12 instructions |
|---|---|---|---|---|
| A | -1.0% | +1.7% | -0.4% | +0.5% / +0.3% / +0.3% |
| B | -1.0% | +1.6% | -0.4% | +0.3% / +0.3% / +0.3% |
| C | +35.4% | +31.7% | +36.7% | +0.4% / +0.3% / +0.4% |

A's sweep (bench/perf/sweep.py, 114 workloads): 29 slower beyond
spread (text/int_fmt 1.071, text/trim 1.058, text/hex 1.052,
httpparse/h1_chunked 1.077, json/invalid 1.029, digest/pbkdf2 1.042),
2 faster. Why: the spills in the big functions come from the real calls
between the drops (x509.parse still saved all ten callee saved
registers and spilled as much), and in string and JSON code most drops
are the last reference, which now saves and restores 16 registers
around every free.

## Alternatives

Fewer values live across the real calls (shorter functions, or the
compiler's own scheduling of releases next to their last use) would
attack the cause; a cheaper free path (a free list push inline) would
make the last reference cheaper on any convention. Neither is started.
