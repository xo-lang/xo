# Mixed-text case mapping: a two-byte decode for the common runes, 2026-10-24

Follow-up to `case-fold-2026-10-24.md` and `casefold-copy-2026-10-24.md`
(2026-10-24). After the fused fold, `lower` and `upper` stood at 0.96 of
the base; the mixed rows (`lower_u`, `upper_u`, 0.59 of Go) were the
remaining cost, so I profiled them first.

## The profile

`XO_PROF=cpu XO_PROF_HZ=1000` on the arm64 build, `xo prof` for the
summary. The runs are short (97 ms of CPU for `lower_u`, 42 ms for
`upper_u`), so the percentages are coarse, but the order is clear:

| frame | lower_u | upper_u |
|---|---|---|
| `unicode.u24` (three checked index reads of a case table) | 34.0% flat | 19.0% flat |
| `unicode.case_map` (r >= 0x800) | 26.8% cum | 16.7% cum |
| `raw.push_rune` to `obj.rs_push_word` (two calls per changed rune) | 24.7% cum, 15.5% flat | 16.7% cum, 7.1% flat |
| `unicode.decode` to `raw.decode` | 16.5% cum, 15.5% flat | 28.6% flat |
| `unicode.to_case` | 12.4% cum | 14.3% cum |
| `obj.fold_copy` (the fused fold) | 4.1% cum | 4.8% cum |

`to_case` already has the direct three-byte table below U+0800, so the
Latin, Greek and Cyrillic ranges the lead asked about were not the
problem; reading that table (`u24`) was.

## What was tried

1. `u24` through the table's address (`raw.bytes_addr` once, then three
   `rt.mem_load` of one byte) instead of three bounds checked index
   reads. Measured worse: `lower_u` 4,947.1 M against the base's 4,596.8
   M (1.076), `eqlower` 12,311.9 M against 11,596.8 M, `runecase` 237.3
   M against 204 M. `bytes_addr` is itself two runtime calls
   (`rt.obj_addr`, `rt.layout`) per lookup, more than the checks it
   saved. Reverted.
2. The two byte decode inline in `case_append`. The loop re-enters only
   at a non-ASCII byte (the ASCII runs are skipped by `scan.span`), so
   `raw.decode`'s general path (bounds checks, the lead byte's four
   ranges, a continuation loop) was doing that work for what is nearly
   always one two-byte rune. Kept.

## What the two-byte decode is worth

Instructions retired, arm64, best of 5, base = today's `origin/main`
built by an xo from the same revision. The base xo is from `e50722a01`
and the change is on `314384917`; the difference between them is Lang
1's `xo test` no-tests notice, no code generation (the lead checked).

| workload | base | now | now / base |
|---|---|---|---|
| lower_u (mixed) | 4,596.8 M | 4,206.8 M | 0.915 |
| upper_u (mixed) | 4,568.9 M | 4,167.1 M | 0.912 |
| eqlower (case compare) | 11,596.8 M | 10,826.7 M | 0.934 |
| runecase (Rune.to_lower/to_upper) | 204.0 M | 200.3 M | 0.98 |
| lower (ASCII) | 392.8 M | 378.2 M | 0.963 |
| upper (ASCII) | 402.5 M | 386.7 M | 0.961 |

The ASCII rows move only by the fused fold from the earlier change,
which is the base here.

## What is left

- `u24` is still three checked reads. The fix that would pay is a table
  layout, not a call: store the below U+0800 case tables little endian,
  so one four byte load and a mask give the value. That needs the table
  generator and its sync test, so it is a separate change.
- `push_rune` costs two calls per changed rune (`raw.push_rune` then the
  `rt.rs_push_word` cabi stub into `obj.rs_push_word`). Encoding the two
  byte rune in `case_append` and calling `obj.rs_push_word` directly
  would drop a hop, if the llrt module may import `obj`.
