# 0184: arm64 inlining of small non-leaf functions, and checks proved by bounds and loop conditions

Status: Accepted (2026-10-12), implemented in `internal/arm64` and its
Xo twin `selfhost/arm64` (byte identical output, decision 0158):
`inline.go` / `inline.xo` (cost model), `opt.go` / `opt.xo` (bounds,
`less` facts, `joinSource`, `threadConst` chains, `liveDCE`),
`ranges.go` / `ranges.xo` (midpoints, Str lengths), `isel.go`,
`isel.xo`, `calls.xo` (checks, byte loads, logical immediates). Extends
decisions 0049 (inlining) and 0054 (value ranges); supersedes the
"bounds checks all stay" parts of 0054, 0060, and 0075. Tests: golden
`bce_inline` (internal/llvm/testdata/golden, Go, LLVM, arm64),
`TestInlineCostModel`, `TestThreadCsetAssumptions` (a move only the
join read is dropped). Evidence: `bench/results/runtime-pilot-2026-10-12.md`,
section "arm64 inlining and bounds checks (decision 0184)".

## Context

Decision 0180's runtime pilot moved the Unicode operations from C to
Xo. LLVM met the bar; the arm64 backend ran per rune work at 1.45 to
1.80 times the C's instructions, because it inlined only leaf functions
of at most 40 SSA instructions (so `Rune.is_letter`, a forwarder to
`unicode.is_letter`, became two calls, and decode, case mapping, and
the table searches stayed calls), and because its loops kept every
check: bounds checks, overflow checks of index arithmetic, and the
checks of `Int(t[i]) << 16`. Decision 0180 item 7 sets the bar for
porting more of the runtime: `bench/perf/unicode` within 5 percent of
`XO_UNICODE=c` in instructions on every workload on both backends.

Decisions 0054, 0060, and 0075 had measured removing loop bounds checks
proved by the loop and kept them (nbody 1.0 to 1.5 percent slower, a
property of the loop's shape). Decision 0004's checked arithmetic must
stay correct: a check is removed only when it is proved.

## Decision

1. **Inlining cost model.** A function's cost is its instruction and
   block count, where a call of an inlinable Xo function counts that
   function's cost and any other call one instruction. A function is
   inlinable when its cost is at most 40 instructions and 12 blocks, or
   when it has at most 2 call sites in the module (its own calls not
   counted), sites times instructions is at most 500, and it has at
   most 80 blocks. Never main, never a function with landing pads of
   its own. Costs come from a depth first walk of the call graph
   (functions in module order, callees in instruction order); a
   function that calls one still being walked (a cycle, or itself) is
   not inlinable, so every cycle has a function that is not and copies
   never nest without end. A forwarder to a large function is a few
   instructions and is inlined; the large function stays a call. Each
   function adds at most its budget of copies, so code grows linearly.
2. **Bounds through operations.** `bounds(v)` intersects the range
   analysis, the memo's branch facts, and an interval derived from the
   operands' bounds three operations deep (add, sub, mul, div and shift
   right by constants, and, or, checked shift left, conversions, the
   length of a constant Str). A branch `x op y` with `x = a + c` also
   bounds a.
3. **Relational facts.** The memo records `x < y` for registers from
   strict compares on branch edges and from passed bounds checks. A
   bounds check (List and Bytes) is dropped when the index is at least
   0 and either below a constant length or at most an x with `x < len`
   known: the same value, the same base plus a smaller offset (`i + 1`
   under `i + k < n` with k at least 1), or the same base plus the same
   offset value. Blocks entered through the Bool join of `a && b` or
   `a || b` take the facts of the one predecessor that can take their
   edge (`joinSource`): its facts only (bounds, relations, passed checks), not the
   registers it computed, which come from the block's dominator
   (2026-10-13, gotchas/arm64-vreg-read-before-write-break-flag.md:
   that predecessor does not dominate the block in the graph, and a
   register it computed was read on a path that never ran it).
4. **Midpoints.** The range analysis gives `m = a + (b - a) / 2` (or
   `>> 1`) computed where `a < b` holds (a dominating block entered only
   by that edge) the interval `[a.lo, b.hi - 1]`: binary searches over
   constant tables lose every check.
5. **Other checks.** A shift left whose operand fits in the bits left
   after the largest shift is unchecked; a variable shift amount in
   `[0, 63]` is not checked; an add or subtract of two registers whose
   bounds prove it is unchecked. Str and Bytes lengths are at most 2^48.
6. **Code shape.** Byte loads compute the data address once per block
   and `t[k + c]` as one add and loads at `#c`; `and`, `orr`, `eor` take
   a run of ones as an immediate; `threadConst` and `threadCset` follow
   blocks that only move registers and jump (the joins an inlined return
   passes through) and copy their moves; `liveDCE`, a flow sensitive
   dead code pass before `selectCompares`, removes what the threading
   left dead (a register live into a landing pad counts as live).

## Addendum: the leaf intrinsics and tail calls (2026-10-25)

**The intrinsic list.** inline.xo's inline_intrinsic (the rt.* names a
function may call and still be a leaf) named 23 of the 100 names
compiler/arm64 calls.xo lowers to instructions. 23 more pure ones are
now listed: rt.bytes_word, the 16-byte compares, clz, ctz and popcount,
the float conversions, mul_hi, list_esize, rs_put_word, rs_set_len,
obj_addr, slot_addr and clmul. internal/repotest
TestInlineIntrinsicsListed keeps the two lists apart: a name calls.xo
lowers must be in inline_intrinsic or in its not-leaf list with a reason
(block kernels, C and system calls, memcpy and the like, retains, the
unwinder, thread local storage).

**Tail calls.** The list alone made unicode classify 1.195 times the
instructions:
- unicode.in_ranges became a leaf, so it became inlinable. The cost walk
  then added its cost into each rune predicate that tail calls it on its
  slow path (is_digit, is_print).
- That pushed those predicates over INLINE_MAX_INSTRS, and classify's
  loop called them, two calls a rune.

A call in tail position is a branch on arm64 (shrink.xo mark_tails).
So in the cost walk and at the call site, a tail call:
- of a callee over INLINE_TAIL_MAX (8) instructions,
- in a caller with more than INLINE_TAIL_MAX instructions of its own,

stays a call and counts as one instruction. The copy of such a caller in
a hot one holds the call where it is no longer a tail call, and the
usual rules take it.

Each condition answers a measured cost:
- Without the caller condition, wrappers such as cabi.ctx_unwinding
  (`return ctx.unwinding()`) kept their callee as a call: callbacks
  any, filter and fold 1.026 to 1.029.
- A variant that also required the callee to be a leaf (C) halved the
  size cost but kept little of the speed. It was measured and not taken;
  see the results.

FnSum records each call's tail flag (INLINE_SUM_VERSION v2), and the
inline cache key carries the set (INLINE_CACHE_VERSION v8).

below (Xo Lead's pick, B, 2026-10-25; the owner may ask for C).

Instructions against the parent's xo, the same source base:
- unicode: classify 0.992, classes 0.928, mapfunc 0.989;
- text: 0.98 to 1.009 (split 0.978);
- json: 0.99 to 1.0;
- callbacks: 0.995 to 1.001;
- crypto: 0.998 to 1.004.

Quiet `xo bench --ab`, wall over A, 20 rounds:

| row | B (taken) | C (leaf callees only) |
|---|---|---|
| unicode classes | 0.802 (0.794 to 0.814) | 0.930 (0.919 to 0.940) |
| unicode classify | 1.007 (noise) | 0.999 (noise) |
| unicode mapfunc | 0.965 (0.95 to 0.98) | 0.993 (noise) |
| text parse_int | 1.010 (noise) | 1.030 (1.005 to 1.052; its instructions 1.000) |
| text split | 0.978 (0.963 to 0.991) | 1.011 (noise) |
| text replace | 0.999 (noise) | 1.008 (noise) |
| xo s2 __text | +277 KB (+0.46%) | +144 KB (+0.24%) |
| bench/perf/http/xo __text | +6.1 KB (+0.4%) | +3.5 KB |

## Consequences

- `bench/perf/unicode` on arm64 is within 5 percent of the C path
  (`XO_UNICODE=c`, same compiler, same run) on every workload, in
  instructions retired: lower 0.67x, upper 0.63x, lower_u and upper_u
  0.88x, eqlower 1.016x, classify 1.027x, runecase 0.89x, quote 0.64x.
  Against the arm64 code before this work: 39 percent fewer
  instructions on mixed script case mapping, 37 on the lower compare,
  29 on the classes, 42 on Rune case mapping, 17 on quoting. arm64 now
  retires fewer instructions than LLVM on lower_u, upper_u, eqlower,
  and runecase. Decision 0180 item 7(b) is met on arm64; on LLVM it is
  not (that backend is unchanged: mixed script case mapping 1.02x, the
  lower compare 1.13x, Rune case mapping 1.17x of the C path in
  decision 0180's run), so porting more of llrt still waits on LLVM.
- Bench programs: no change beyond run to run noise (instructions
  within 0.3 percent, cycles within 3 percent) except sort, 1.4 percent
  fewer instructions; nbody loses 2 of its 11 bounds checks and its
  best cycles were 0.6 percent above the before binary's (inside the
  noise of the run; decision 0075's placement sweep would separate a
  shape effect from placement).
- xo1 (the self hosted compiler) compiling itself: 1,575.5 billion
  cycles and 3,857.4 billion instructions, against 2,015.5 billion and
  3,998.7 billion before (one run each, different load), while writing
  8.5 percent more assembly; its binary is 8.4 percent larger (the
  copies of the few call sites rule). The fixed point of decision 0158
  holds (Go emitter, xo1, and the rebuilt xo1 write byte identical
  assembly of selfhost/xo).
- More functions are inlined, so frames of callers grow (nbody's main
  saves 6 more registers once); golden assembly of sieve changed (an
  `n + 1` the loop exit proves is unchecked).
- Facts are per register in the memo (as the existing bounds), so they
  hold where the register's definition dominates; `liveDCE` must stay
  before `selectCompares` and `reuseFlags` (gotchas
  arm64-reused-flags-of-dropped-compare.md).

## Alternatives

- **Inlining to a fixed point with the leaf rule** (decision 0180's
  experiment): 0 to 11 percent, not enough; the size limit, not the
  round count, was the obstacle.
- **A larger limit for every function** instead of the few call sites
  rule: grows code with every call site; the decoder and the case
  mapping (180 and 241 instructions) are called from two loops each.
- **Inlining only call sites in loops**: needs loops at the SSA level
  and does not bound copies of a function whose callers are inlined
  again; not built.
- **An ABCD style inequality graph** for the binary search: general,
  but the one pattern the runtime needs (the midpoint) is a local rule
  of the range analysis.
- **Changing unicode.xo** (direct tables for three byte runes, an
  inline UTF-8 encoder): out of scope; the backend change helps every
  program.
