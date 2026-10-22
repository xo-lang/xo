# Views in raw.str_slice: split, lines, trim, trim_prefix/suffix (2026-10-22)

Decision 0330. `raw.str_slice` (the one copying slice of the runtime and
std: `scan.split`, `scan.lines`, `strings.trim`, `trim_prefix`,
`trim_suffix`, http routing, toml, regexp) now returns the language's
`s[lo..hi]` view when both ends are on UTF-8 boundaries.

Native arm64, instructions retired (`/usr/bin/time -l`), programs
`mb/sliceb` (Xo) and `mb/slicebgo` (Go twin, strings package). Before is
main at 099921838, after adds the change.

| mode (work) | before | after | change | Go |
| --- | --- | --- | --- | --- |
| split (200k x 12 fields) | 515.1 M | 446.1 M | -13.4% | 403.8 M |
| lines (200k x 4 lines) | 207.3 M | 178.5 M | -13.9% | 223.9 M |
| trim (400k x 3 calls) | 331.5 M | 295.5 M | -10.9% | 201.2 M |
| trim_prefix/suffix (400k x 2) | 204.3 M | 182.6 M | -10.6% | 50.5 M |

The gap that remains on trim and trim_prefix/suffix is the view's own cost
here: the boundary test and the slice's range check run twice (once in
`raw.str_slice`, once in `s[lo..hi]`), and the object gets a count. The
callers below know their ends are boundaries; an unchecked view needs a new
runtime intrinsic, which waits for a seed with 0330 (gotchas/seed-older-than-intrinsics.md).
