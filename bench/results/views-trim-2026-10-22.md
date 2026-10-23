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
