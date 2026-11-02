# debug.mem_trim drops the large block cache (decision 0448 case (c))

Last verified: 2026-11-02. Base 083fea894. linux/arm64 rows through ab-host (first host, `edit`: the warm rebuild, 3 rounds each); Mac rows through `xo bench --ab`, 3 rounds, coll's warm build.

## Every cached mapping dropped, three phase ends

| Program | Wall | Max RSS |
|---|---|---|
| coll edit, 4 runs | 1.008, 1.023, 1.032, 0.999 | 0.918, 0.951, 0.895, 0.894 |
| self (2 runs) | 1.003, 1.002 | 1.002, 1.009 |

A/A of coll edit, one revision against itself: wall 0.998, RSS 0.981. Rejected: over the 1% wall limit in two of four runs.

## Every mapping, fewer phase ends on Linux

| Trim on Linux | Program | Wall | Max RSS |
|---|---|---|---|
| emit, link | coll edit | 1.012, 0.993, 1.000 | 0.931, 0.898, 0.932 |
| emit, link | conc-spawn edit | 1.033 | 0.899 |
| emit | conc-spawn edit | 1.038, 1.031, 1.021 | 0.902, 0.899, 0.900 |
| emit | coll edit | 0.999, 1.009, 1.007 | 0.876, 0.931, 0.933 |

Rejected: conc-spawn stays about 3% slower, the link refaulting pages it would have reused.

## Mappings of 2 MiB and over, three phase ends (landed)

| Program | Wall | Max RSS |
|---|---|---|
| coll edit, 3 runs | 0.991, 0.992, 1.001 | 0.902, 0.920, 0.936 |
| conc-spawn edit, 3 runs | 1.008, 1.010, 0.993 | 0.888, 0.974, 0.951 |
| coll edit, final tree | 0.999 (0.999 to 1.015) | 0.917 (0.900 to 0.936) |
| self, final tree | 0.997 (0.994 to 0.999), CPU 0.999 | 0.999 (0.999 to 1.004) |

Mac, coll's warm build, 4 runs: wall 1.028, 1.019, 1.001, 0.989 (each with a spread of about 2%), instructions 1.002, 1.000, 1.001, 1.001, max RSS 0.916, 0.904, 0.895, 0.913.

Mac rows, 3 rounds, RSS / footprint MB, before to after: coll warm 51/32 to 45-46/30-31, edit 51/31-32 to 46-47/30-32; conc-spawn warm 44-45/27-28 to 40/27, edit 43-44/27 to 39-41/27. On darwin most of this is the RSS reading (the pages were already reusable, outside the footprint); on Linux, which advises nothing, it is real memory.
