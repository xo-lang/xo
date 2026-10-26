# Functions that return a view of their argument or a fresh copy (2026-10-25, Xo Lang 3)

Question (the lead, from DS-2's view item): does an interprocedural "returns a view of
parameter i" summary pay? Static reach first, then the cheaper source level alternative.

## Reach (counter: ~/xo-lead/notes/lib1/borrow-summary-count.py on the rc stage SSA and the final assembly)

Sites left after inlining, by the class of the callee (all returns a parameter = ident, a view
= view, in an Option = wrapped, some = partial):

| program | ident | view | wrapped | partial |
|---|---|---|---|---|
| bench/perf/text | 0 | 0 | 0 | 21 |
| bench/perf/http/xo (CRUD) | 0 | 4 | 1 | 163 |
| compiler/xo | 0 | 59 | 14 | 1199 |

A pure summary reaches 77 sites in compiler/xo and 5 in CRUD, none hot. The mass is partial
functions (a view on the common path, a copy on a rare one): raw.str_slice 408 sites in xo,
strings.trim 203, str_trim_prefix 165, str_trim_suffix 73, map_case 65.

## What paid

raw.str_slice (40f67b3a2): 62 instructions to 8 (Int.max/min clamps, an unchecked rt.str_view
tail, the copy of a cut inside a UTF-8 sequence in raw.str_cut_copy, a cold_fn call). Calls of
str_slice 393 to 0 in xo, 57 to 0 in CRUD. Sweep: text/lines 0.969, text/replace 0.967,
text/split 0.994, stdio/err 0.993 faster; text/trim 1.007 slower. __text: CRUD +0.08%, xo +0.02%,
hello -96 B. (Its --ab wall row is pending.)

## Do not retry (measured, reverted)

1. str_cut under the inliner's small limit (40 instructions, 12 blocks): it is 43 and 19 blocks
   as written; two early returns 43/15; `lo < hi` for the lower test 43/19. The mid_rune helper
   is 8 instructions inlined twice. It inlines only in loops (369 calls in xo remain).
2. A fast path in str_trim, str_trim_start and str_trim_end (the end byte outside 33..126 goes to
   strings.trim; a branch free two byte test with min/max, 35 instructions, inlines): bench text/trim
   is all padded strings, so it always takes the slow path: 2,944 M to 3,065 M instructions
   (+4.1%). No bench trims unpadded text, so the saving (the retain/release pair and the call,
   about 26 instructions) has no row. Needs a bench of typical inputs first.
3. scan.eq_text as a call (cold_fn) so str_trim_prefix and str_trim_suffix shrink: size -11.5 KB
   in xo, but a trim_prefix plus trim_suffix hit/miss loop went 461 M to 516 M (+12%): the
   inlined word compare beats the call, and the wrappers still do not inline (suffix 53
   instructions, 19 blocks).
4. A summary of the callee's result as a borrowed view (BodyFacts, cache key, ABI of the fault
   paths): reach is the table above; not built.
