# 0376: A function's result bounded by its parameters, summarized for its callers

Status: Accepted as built (Xo Lang 3, 2026-10-25, at the lead's request
after the dur_parse bounds checks of 3a7059189: "the range pass can't carry
'the result is within [lo, hi]' out of a call like space_start").

## Context

`strings.space_start(b, lo, hi)` returns an n with lo <= n <= hi, and
`parse_duration` tested `i < n` for that n, then read `b[i]`. The native
range pass relates no value across a call, so each read kept its bounds
check (nine sites, and an address recomputation at each): bench/perf/text
dur_parse was 1.093 of LLVM's instructions. LLVM inlines the helper and
proves the reads. 3a7059189 worked around it with `raw.byte_at` reads under
the contract; this decision is the general fix, which the lead asked for.

## Decision

1. A summary per function (compiler/ssa retle.xo): its one Int result is at
   most parameter k (`ret_le`), and at least parameter j (`ret_ge`), or -1.
   The result is the parameter, a block parameter all of whose incoming
   values are, or one of those less a nonnegative value or plus a
   constant at most 0 (for at most), plus a nonnegative value or less a
   constant at most 0 (for at least); a nonnegative value is a constant or
   a mask by one. An optimistic fixed point over the block parameters
   handles loops. Only the function's own body is read, never a callee's
   summary, so a summary is a function of one body.
2. The summary is kept where a body's facts are: `BodyFacts.ret_le` and
   `ret_ge` (encoding `xo body facts v2`, inline cache v9), so a function
   left undecoded by the cache still gives it (Module.ret_bounds).
3. The summary is part of every caller's function cache key: func_sig of
   each named callee carries `retle=le,ge` (fn cache v9). A caller's text
   reads it (below), so a callee edit that widens its result makes the
   caller generated again. TestBackendCacheCalleeSummary is the test: a
   cached build equals one with the cache off after each step of
   bounded, bounded again, bounded undecoded, widened, reverted; with the
   summary left out of the key it fails at "widened".
4. The emitter uses it three ways (compiler/arm64): a call result's
   interval is [lo of the argument it is at least, hi of the argument it is
   at most] (ranges.xo call_bound), so `x + n` and the like lose their
   overflow checks; `i < n` is also recorded against each value n is at
   most (opt.xo compute_ub, less_ub: n a call result or a block parameter
   all of whose incoming values are one), so `b[i]` under `i < n` for
   n <= b.len() needs no check; and three small relations the same reads
   needed: `x != c` at an end of x's interval (a length `!= 0` is at least
   1), `a - b == c` with c >= 1 gives `b < a`, and `a - c` is below the
   length register a.

## Consequences

- Measured against the pushed head 3a7059189 with one std on both sides
  (2026-10-25): compiler/xo built --release has 2662 bytes-index checks
  (was 2762), 19979 list index checks (20499) and 14959 overflow faults
  (14980), __text 60,847,924 (60,892,492); bench/perf sweep, 134
  workloads: 7 faster beyond spread (text/cut 0.982, parse_int 0.980,
  dur_parse 0.993, int_fmt 0.994, replace 0.994, fsproc/none 0.992,
  portable/str_hash 0.995), none slower (atomic/contended 1.115 on the
  first run is its own contention noise: 1.003 at 7 rounds). Correct on
  arm64 (382 goldens, 30 ssa and 50 arm64 xo tests, codegen patterns,
  the full internal/arm64 package), the fixed point (s1, s2, s3) and
  Docker amd64 (49 of 49).
- The raw reads of 3a7059189 stay: with the summary alone dur_parse is
  3.350 G instructions, with the raw reads 3.249 G, because a raw read also
  hoists the data address (`pa`) out of the loop. unit_of's three reads are
  a precondition of its caller (us < i <= n), not a relation inside it.
- A summary only covers a result that is an Int at most or at least one
  parameter; a result bounded by a constant or by two parameters
  (`min(a, b)`) is not recorded.
- The summary of a function the cache returns undecoded comes from its body
  facts, those of a decoded one from its body: both are the same function
  of the same body, and the key is computed from the same source as the
  emitter's reads (Module.ret_bounds).

## Alternatives

- Only the register level fact (the first prototype): a block parameter
  n = phi(bl, space_start(...)) carries no register fact; SSA-level bounds
  (compute_ub) do.
- Inlining space_start and its loops: grows every caller, and the inliner's
  model refuses a function with a loop.
- A `pub` annotation on the function (`@returns_le(hi)`): checked by
  nothing; the analysis proves the claim.
