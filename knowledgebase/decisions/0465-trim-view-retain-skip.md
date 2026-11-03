# 0465: a Str view that is only measured or compared does not retain its text

Status: Proposed (draft by Xo Lang 3, 2026-11-03; implementation for Lang 1, who owns insert_rc)

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
