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

In `insert_rc`, a view made from a Str that is live for the view's whole lifetime (the
source is owned by the caller and not released between the view's creation and last use)
is not retained, and its release is not emitted. The common cases: trim, slice and split
results used only inside a statement or a block (compared, measured, written, hashed,
copied).

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
