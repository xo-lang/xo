# 0465: a Str view that is only measured or compared does not retain its text

Status: Accepted (owner, 2026-11-03). Drafted by Xo Lang 3 (2026-11-03); implementation by Lang 1, who owns insert_rc: step 1 (view_proj, opt-in) in 13a19d72f.

## Context

bench/results/std-twin-rows2-2026-11-03.md: `Str.trim_chars` 2.24, `trim_start_chars` 1.91,
`trim_end_chars` 1.98 of Go's wall. The profile (100,000,000 trims, host 1) is the loop
at 62% and `drop Str` at 24%: since decision 0330 a trimmed Str is a view that holds a
reference to its text, and every view is retained when made and released when dead. Go's
substring is a pointer and length with no count.

The scan itself (llrt/scan.xo trim_chars: Go's one byte, ASCII bitmap and unicode paths)
is already at parity.

## Decision (proposed)

Mechanism (Xo Lang 1, who implements it, 2026-11-03). The retain is not in
the caller: `trim_chars` builds its view with `rt.str_view(s, lo, hi)` inside
llrt/scan.xo, which counts s's buffer, and the caller receives an owned Str
that it drops. So insert_rc in the caller has no retain to skip on its own.
Three steps, in this order:

1. **XO0915 covers views first.** Decision 0460's lint (a raw address used
   after its value was released) is extended to a Str or Bytes view used
   after the release of the value it views, the exact bug a wrong skip
   below would create. It lands, with std clean under it, before any skip.
2. **"Returns a view of p" inference plus a no-retain variant.** Like
   rc.xo's borrow inference, a function whose every return is p itself,
   `rt.str_view(p, ..)`, or a call of another such function on p is marked
   as returning a view of its parameter p (trim_chars, trim_start_chars,
   the slice and cut helpers). insert_rc then calls a second body of it,
   built like rccache.xo's borrowed-parameter shells (lazy_shell), whose
   views do not count the buffer, wherever the caller's p outlives the
   result and the result does not escape (borrow.xo's liveness test of
   decision 0186, with the call's result as the projection and p's leaf as
   its owner). Escaping keeps the counted body: stored in a struct or list,
   returned, sent on a channel, captured by a closure, or passed to a call
   that may store it.
3. **Inlined calls.** Where the call is inlined, `rt.str_view(s, ..)` is
   visible in the caller and is treated as a projection of s, so
   borrow_projections (decision 0186) drops its count with no variant.

Gates: XO0915 clean over std (cmd/xo TestVetCostStd), TestRaceStdNoUseAfterFree,
the 2026-11-02 race stress, and `xo bench --ab` on the trim rows. The default
on push waits for the owner's acceptance.

## Step 1 landed, opt-in (2026-11-03, Xo Lang 1)

`XO_PASSES=+view_proj` (off by default while this decision is Proposed;
native arm64 and amd64 only, since wasm copies a slice): rc.xo treats
`rt.str_view` as a projection of the Str it views (kind(), as list.get), and
borrow_views emits it as `rt.str_borrow`, which the arm64 and LLVM emitters
lower without the count; insert_rc retains the view where it must be owned and
borrow_projections leaves it uncounted where the source outlives it. XO0915
(cost.xo freed_addresses) checks an uncounted view against its source's
release until the view is retained. Four places keyed on the old shape had to
follow, each found by a failure: the verifier's own Rc (verify.xo; rc.xo
kind() treats rt.str_borrow as a projection with no flag), the inliner's
leaf list (inline.xo inline_intrinsic) and cost (inlsum.xo fn_sum: a Retain
right after its rt.str_borrow is not counted; scan.trim_chars went from 149 to
151 instructions over INLINE_ONE_PADDED_INSTRS), and view_lens (viewlen.xo, a
view only measured is its length); gotchas/rc-pass-shape-crosses-inline-limit.md.
Folding an owned view's Retain back into a counted rt.str_view broke the
inlined module's verification of xo itself and was dropped.

Gate (host 2, linux/arm64, every arm64 golden and the vet cost, race
use-after-free, LLVM string and repo tests with the pass on): pass; xo builds
itself with it on by default (the measure commit). `xo bench --ab` on the same
host, on against off by default (15 rounds): a program of views made and
compared in place (inlined slices) 0.92; trimchars trim 0.975 and trim_start
0.989 (within noise), trim_end 1.018 (within noise, 21 rounds) and 1.035 with
the sides' order swapped. Step 1 alone does not reach the trims' `drop Str`:
trim_chars returns its view, so the count stays; that is step 2's variant.

## Default on (2026-11-03, Xo Lang 1)

After the owner's acceptance, view_proj is on by default on the native
targets (rc.xo view_proj_on: unless `XO_PASSES=-view_proj`; wasm stays off,
since it copies a slice). Gate under the defaults rule, on the first
linux/arm64 offload host, 2026-11-03: scripts/gate.sh arm64 llvm cmd fp
with XO_GATE_LLVM=1 (the arm64 goldens, the full LLVM goldens and the -O2
sample, the full cmd/xo suite, the fixed point, chkcache, cross) all
pass, after xo test compiler/ssa compiler/arm64 (107 passed).

## Step 2 landed (2026-11-03, Xo Lang 1)

Built without a second body: the calling convention itself changes for a
function that qualifies, since every caller of such a function is known.
ssa/retview.xo `return_views`, a pessimistic fixed point after the borrow
inference, marks a function as returning a view of parameter p
(`Func.ret_view`, `Rc.rvp`) when every Return is p itself (the entry
block's parameter, not a join's), `rt.str_view`/`rt.str_borrow` of such a
value, a call of another marked function on its p, or a constant (an
immortal Str, a fault path's zero value), p is borrowed, its one result
is a Str, and it is reached only directly: not indirect, an entry, an
export, or the target of a FuncAddr or Closure. Then:

- the callee returns the view uncounted (insert_rc's Return borrows its
  argument; modes() agrees for the verifier);
- each caller treats the call as a projection of its argument p (rc.xo
  kind() and parent(), as rt.str_view under view_proj), so insert_rc
  retains the result only where it must be owned (stored, returned,
  passed to a consuming call, kept past p's release) and
  borrow_projections leaves it uncounted elsewhere;
- std's facts carry it (std facts v3, `StdFact.ret_view`; a mismatch is a
  calling convention error in the facts verifier), and extern shells take
  it from them;
- such a function inlines only with exactly one Return (arm64/inline.xo
  inline_call `direct`): the call's uses take the returned view itself,
  with no join parameter, which the counting would own. With more than
  one it stays a call (a join would return a counted value into a caller
  that expects a view). The count of Returns is the inliner's summary's
  (inlsum.xo FnSum.rets, inline sum v3), which a warm build reads from
  the cache, so cold and warm builds decide alike
  (gotchas/inline-shell-has-no-blocks.md). The rc, inline and function
  caches key on the fact (rc cache v13, inline cache v12, fcache's
  signature line);
- kind() asks for every call value, so the names of the marked functions
  are kept apart (`Rc.rvn`) and looked up only for a Str result; the
  verifier's checker (verify.xo rc_checker) builds them too
  (golden ret_view_field_arg; without them xo's self build failed under
  XO_INLINE_STRICT=1);
- std's trims qualify: llrt strings.trim cuts with rt.str_view (a Str is
  valid UTF-8 and both scans step whole runes, so raw.str_cut's copy
  never ran) and the trim_chars family returns rt.str_view(s, 0, len)
  where it returned s;
- XO0915 follows the convention: a marked function's result is checked
  against its argument's release as an rt.str_borrow is.

`XO_PASSES=-ret_views` turns it off; it is on with view_proj. Three
places held a copy of the function without the field and failed only on a
warm build (gotchas/func-field-lost-on-cached-build.md). Gates and the
measurement: on the second linux/arm64 offload host, 2026-11-03,
scripts/gate.sh arm64 llvm cmd fp with XO_GATE_LLVM=1 (arm64 goldens,
the full LLVM goldens and the -O2 sample, cmd/xo, the fixed point,
chkcache, cross) all pass, after xo test compiler/ssa compiler/arm64
(108 passed) and internal/arm64 TestStdFactsVerify, whose Go decoder of
the std facts now reads the ret_view field. At the final revision (step
2 rebased onto 1e5f98fe6) on the same host: xo test compiler/ssa
compiler/arm64 (108 passed), gate.sh arm64 fp (arm64 goldens, the fixed
point, chkcache, cross) pass; the LLVM goldens and cmd/xo passed at its
parent a8b75ae1, whose rebase brought in no compiler/llvm or cmd change
of its own. Two goldens failed once each under the gate's load and pass
alone at the change and at main (http_reverse_proxy_ws 25 of 25,
TestStdRecordDefaultArgs, and both again in a rerun).

Measured with `xo bench --ab` on the first linux/arm64 offload host,
2026-11-03, A = main, B = step 2 (bench/results/retview-step2-2026-11-03.md):

- generated code: bench/perf/trimchars trim 0.741 (0.663 to 0.76),
  trim_end 0.728 (0.697 to 0.744); bench/perf/retview use 0.721 (0.72 to
  0.734), keep 0.746 (0.727 to 0.845); bench/perf/jitter's server
  (scripts/release-vs-go-http.sh, base = main's xo, 4 pads): every row
  within placement (req/s 0.998 to 1.007);
- xo's self build, both xo built by the seed (step 2's compile cost):
  wall 1.007 (0.997 to 1.010), cpu 1.003 (0.999 to 1.006), within noise;
- xo's self build, each xo built by itself (s2, so B's own code carries
  the views), XO_INLINE_STRICT=1, 8 rounds from a quiet host: wall 1.004
  (0.999 to 1.009), cpu 1.004, max RSS 1.002, all within noise. One
  binary building xo with the pass on and off: insert_rc +44 ms, emit
  -305 ms.

An earlier lookup (a hash of the callee's name in kind() for every call)
cost the self build 1.1%; a later draft read 0.94 because its xo failed
the inlined module's verification and built without inlining
(gotchas/ab-self-measures-inline-fallback.md).

## Consequences

- Removes the `drop Str` share of trim and slice heavy loops (24% of this profile).
- Touches insert_rc, next to decision 0460's use-after-free class (a raw address outliving
  its value): a borrowed view is exactly that shape, so the pass needs the XO0915 lint
  extended to views and the race stress (the 2026-11-02 five finding stress) as a gate,
  and the RC owner (Lang 1) as implementer.
- The view must not escape: stored in a struct or list, returned, sent on a channel,
  captured by a closure, or passed to a call that may store it keeps the retain.

## Alternatives

- Make `trim_*` return a fresh copy: removes sharing, costs an allocation, slower.
- Do nothing: Early on the performance axis for the prelude.
