# The empty-separator split sized from the rune count, and the sizing that was reverted, 2026-10-24

Program: `bench/perf/strsplit` (eleven workloads, each printing a
checksum) and its Go twin `bench/perf/strsplit/twin.go.txt` (the same
workloads with `strings.Split`, `strings.SplitAfter`, `strings.Fields`,
`bytes.Split` and `bytes.Fields`, and a `lines` helper written to the
documented `Str.lines` semantics). Instructions retired
(`/usr/bin/time -l`, best of 5), arm64 native and an LLVM -O1 build of
the same program. Every workload's checksum is identical on arm64, llvm
and Go (and the base and new binaries print the same checksum).

The change: `Str.split("")`, `Str.split_n("")`, `Bytes.split("")` and
`Bytes.split_n("")` size their piece list once from the rune count
(`scan.bytes_rune_count`, capped by n, as Go's explode does) instead of
growing it, and make each piece with `rt.str_view` (the bounds are UTF-8
boundaries by construction) instead of the checked `raw.str_slice`.

## Base against new, arm64 native

Base = the program built with the parent commit's std (`XO_STD` pointed
at `git archive origin/main std`), new = the same program built with this
change. Instructions retired, best of 3.

| workload | base | new | new / base |
|---|---|---|---|
| split (5 separators in turn) | 279.2 M | 176.0 M | 0.63 |
| split_one (",") | 120.9 M | 120.9 M | 1.00 |
| split_multi ("abc") | 137.2 M | 137.0 M | 1.00 |
| split_absent ("§", no match) | 91.2 M | 91.0 M | 1.00 |
| split_empty ("") | 841.6 M | 327.2 M | 0.39 |
| split_after (",") | 158.5 M | 158.6 M | 1.00 |
| lines | 111.6 M | 111.6 M | 1.00 |
| bytes_split (",") | 154.4 M | 154.3 M | 1.00 |
| bytes_fields | 242.9 M | 242.8 M | 1.00 |
| fields | 234.8 M | 233.5 M | 0.99 |
| index (the loop alone, no split) | 41.45 M | 41.44 M | 1.00 |

Only the empty separator, where there is one piece per rune, gains; the
`split` row gains because one of its five separators is "".

## Against Go, the final tree

| workload | arm64 | llvm | Go | arm64 / Go | llvm / Go |
|---|---|---|---|---|---|
| split (5 separators in turn) | 176.0 M | 175.6 M | 206.9 M | 0.85 | 0.85 |
| split_one (",") | 120.9 M | 129.4 M | 172.4 M | 0.70 | 0.75 |
| split_multi ("abc") | 137.0 M | 141.2 M | 153.4 M | 0.89 | 0.92 |
| split_absent ("§", no match) | 90.9 M | 96.6 M | 110.1 M | 0.83 | 0.88 |
| split_empty ("") | 327.3 M | 298.7 M | 367.0 M | 0.89 | 0.81 |
| split_after (",") | 158.3 M | 161.5 M | 171.6 M | 0.92 | 0.94 |
| lines | 111.6 M | 117.2 M | 144.5 M | 0.77 | 0.81 |
| bytes_split (",") | 154.2 M | 136.5 M | 229.2 M | 0.67 | 0.60 |
| bytes_fields | 242.8 M | 230.7 M | 393.4 M | 0.62 | 0.59 |
| fields | 233.6 M | 229.7 M | 314.5 M | 0.74 | 0.73 |
| index (the loop alone, no split) | 41.7 M | 31.6 M | 22.9 M | 1.82 | 1.38 |

The control row is Go's own loop: Xo's loop and two length reads cost
1.8 times Go's on arm64 and 1.4 on LLVM, so the net figures are lower
than the raw ratios in every row (for example split_one: 79.2 M against
149.5 M, 0.53, and fields: 191.9 M against 291.6 M, 0.66).

## The sizing that was measured and reverted

Counting the pieces first (Go's `Count+1`, `make([]string, 0, n)`, or a
counting pass before `Fields`) was implemented for the other forms,
measured, and reverted. Instructions retired, best of 3, base against
that version:

| workload | base | count first | ratio |
|---|---|---|---|
| split_one (",") | 120.9 M | 129.7 M | 1.07 |
| fields | 234.8 M | 353.4 M | 1.50 |
| bytes_fields | 242.9 M | 362.3 M | 1.49 |
| split_after (",") | 158.5 M | 173.8 M | 1.10 |
| lines | 111.6 M | 134.9 M | 1.21 |
| bytes_split (",") | 154.4 M | 169.4 M | 1.10 |

The reason: a list's growth doubles, so it copies O(pieces) words in
total, while the counting pass is a second walk over every byte (and for
`fields` a second decode per field). Go's `Count+1` wins there because
Go's count is a `bytealg.CountString` that is cheaper than the split
loop, and `Fields` counts with an ASCII fast path; in Xo both passes cost
about the same as the split itself.

## `xo bench --ab`

Quiet window, 2026-10-24 06:52 to 06:56 local, load 3.2 to 3.4, 12 rounds
per workload, A = the parent commit's std, B = this tree, arm64 native
(`xo bench --ab base new --rounds=12 -- WORKLOAD`):

| workload | wall B/A | instructions B/A | verdict |
|---|---|---|---|
| split | 0.741 (0.713 to 0.766) | 0.631 | better, both |
| split_empty | 0.526 (0.520 to 0.532) | 0.389 | better, both |
| split_one | 0.988 (0.971 to 1.013) | 0.999 | within noise, wall; instructions 0.999 |
| fields | 0.991 (0.971 to 1.025) | 0.995 | within noise, wall |
| bytes_split | 1.006 (0.985 to 1.015) | 1.000 | within noise, both |

The two rows the change is for are better by half to a quarter of the
wall time and to 0.39 and 0.63 of the instructions; the three rows that
do not take the empty-separator path are unchanged, as they should be.
