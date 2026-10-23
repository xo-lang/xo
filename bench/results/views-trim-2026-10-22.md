# Unchecked view for trim and trim_prefix/suffix (2026-10-22)

Follow-up to views-split-2026-10-22.md. `raw.str_cut` (std/native/llrt/raw.xo)
replaces `raw.str_slice` in `strings.trim`, `str_trim_prefix` and
`str_trim_suffix`: the caller's range is known inside the string, so there is
no clamp and no second range check, and the view is `rt.str_view`, declared in
std/native/rt.xo. The emitters lowered `rt.str_view` since decision 0330 step 3
(in the 0.25.0 seed), so no new intrinsic and no new seed: the 0.25.0 seed
builds compiler/xo with the edited std.

Native arm64, Mac, `xo bench --ab --rounds=10`, `mb/sliceb` before (origin/main
std) against after, same binary otherwise:

| mode | wall | instructions | before | after | Go |
| --- | --- | --- | --- | --- | --- |
| trim (400k x 3) | 0.835 (0.779 to 0.89) | 0.710 | 296.0 M | 210.3 M | 201.2 M |
| trim_prefix/suffix (400k x 2) | 0.816 (0.768 to 0.904) | 0.676 | 182.9 M | 123.7 M | 50.5 M |

The estimate in views-split (a few percent) was low: the double check was most
of the remaining cost. trim is now within 5% of Go's instructions.

## split and lines (same day)

`scan.split` and `scan.lines` take `raw.str_cut` the same way (every end is a
match or the string length). Same method, `mb/sliceb`:

| mode | wall | instructions | before | after | Go |
| --- | --- | --- | --- | --- | --- |
| split | 0.783 (0.705 to 0.841) | 0.572 | 446.6 M | 255.4 M | 403.8 M |
| lines | 0.774 (0.704 to 0.79) | 0.652 | 178.8 M | 116.6 M | 223.9 M |

Both now execute fewer instructions than Go. Output is byte-identical (md5 of
the program output, before and after).

## trim_prefix and trim_suffix (same day)

`trim_affix` (400000 iterations of `s.trim_prefix("Bearer ").len() +
s.trim_suffix("nop").len()`), same method. The byte loop with bounds checks in
`eq_at` for 3 and 7 byte affixes was the largest part; a 4 to 7 byte overlapping
compare alone gave 0.867 of the instructions, the text compare without Bytes
values 0.725, and the one-test cuts 0.684.

| mode | wall | instructions | before | after | Go |
| --- | --- | --- | --- | --- | --- |
| trim_affix | 0.689 (0.646 to 0.853) | 0.684 | 124.0 M | 84.8 M | 50.5 M |

Still 1.7x Go's instructions: the three calls (compare, cut, release) are not
inlined, where Go inlines HasPrefix. Output is byte-identical.

## trim_prefix and trim_suffix inline (2026-10-23)

The native inliner's report (`XO_INLINE_REPORT=1`) showed `str_trim_suffix` as
"no (pads)": the checked subtraction and adds in the compare and cut helpers
gave it landing pads, and a function with pads is inlined only at a closure
site (or at its one site). With `rt.int_wrap` for the ranges the callers prove,
no length check in the copy of a Str's own range, and the word loop in
`eq_words`, it is "few" (inlined at loop sites).

| mode | wall | instructions | before | after | Go |
| --- | --- | --- | --- | --- | --- |
| trim_affix | within noise (0.921) | 0.895 (0.892 to 0.897, `--ab` n=10) | 84.8 M | 75.8 M | 50.5 M |

1.5x Go's instructions remain: `scan.eq_text` (120 instructions with its
branches and `eq_words`) is still a call per trim, where Go folds the compare
of a constant affix. Output is byte-identical.
