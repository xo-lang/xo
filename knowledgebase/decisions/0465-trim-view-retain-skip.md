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
