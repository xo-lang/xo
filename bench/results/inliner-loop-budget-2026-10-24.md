# Inliner loop budgets for unicode classes: a negative result (2026-10-24)

Xo Lang 1. After DS-1's in_ranges rework (rune-class-index-2026-10-24.md)
sped up LLVM's build more than native's, bench/perf/unicode classes was
1.583 of LLVM's instructions on native arm64 (main b90df01a6, the
native-vs-LLVM sweep of that evening). XO_INLINE_REPORT names the cause:
`class_bits`, the bench's per-rune function, is 622 SSA instructions once
the Rune predicates are inlined into it, so it is never inlined into the
classes loop; inside it, its 12 calls of `unicode.in_ranges` (47
instructions, inlined only at loop sites) and of `unicode.is_print` (63,
over INLINE_MAX_INSTRS 40) are not loop sites and stay calls. LLVM inlines
them all.

Three compiler variants, each against the same main, instructions retired
(unquiet, 3 rounds; the outputs equal) and xo's own __text (s2 of the fixed
point):

| variant | classes | latin1 | classify | fieldsfunc | other rows | xo __text |
|---|---|---|---|---|---|---|
| (1) INLINE_LOOP_SITES 8 -> 16 | 1.0 | 0.999 | 1.0 | 1.0 | 1.0 | +28,312 (+0.05%) |
| (2) (1) and INLINE_LOOP_INSTRS 60 -> 72 | 1.0 | 1.0 | 0.908 | 0.988 | 1.0 (bytes_split 0.999) | +133,824 (+0.22%) |
| (3) a hot function's calls are loop sites | 0.922 | 0.985 | 1.0 | 1.0 | 1.0 (text trim 0.999) | +119,948 (+0.20%) |

The other rows: unicode upper, callbacks for and two_for, json small and
maps, text trim and time_fmt, strsplit bytes_split.

(3) is branch xo4-hotinl (refs/recover/2026-10-24/hotinl, 02b3de008): a
function whose every call site is inside a loop of its caller (FnSum's
loop_calls, inline sum v2) has its own calls treated as loop sites by
find_loop_call, except in its cold blocks (those a branch on rt.landing
enters, and the blocks only they reach); the flag is in the inline key
(cache v8). Even so, only 8 of class_bits' 12 in_ranges sites fit the
loop-site budget, and is_print stays over the loop size limit, so classes
moves from 1.58 to about 1.46 of LLVM's instructions.

None was landed: the gains are one or two rows for 0.05 to 0.22% of xo's
code. The fix taken instead is std-side: in_ranges and is_print split into a
fast path under the inliner's limits (40 instructions, 60 at loop sites)
and an out-of-line rest (DS-1, unicode.xo).
