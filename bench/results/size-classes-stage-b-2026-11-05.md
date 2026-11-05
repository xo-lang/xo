# Size classes four a doubling above 1 KiB (decision 0506 stage B, 2026-11-05)

bench/perf/sizewaste (COUNT byte buffers of SIZE bytes, about 195 MB of payload, every
byte written, held; RSS growth from /proc/self/status), built for linux/arm64 on the Mac
and run on linux/arm64 host 2, one process per cell, one run each (RSS is deterministic
here, as in size-class-waste-2026-11-04.md). Ratio is RSS growth over payload; Go's was
1.00 to 1.08 in that report. Columns (the variants' hashes are WIP builds kept on the
author's branches, not on main; D is the change as pushed):

- main: a27137842.
- B: four classes a doubling from 1 KiB to 16 KiB (d313ab45c).
- D (shipped): four a doubling to 8 KiB, 8 to 16 KiB one page class as on main (b930ab894).
- E: D plus a 10 KiB class (81719d2f9).

| payload bytes | blocks | main | B | D | E |
|---|---|---|---|---|---|
| 1,500 | 133,333 | 1.41 | 1.06 | 1.06 | 1.06 |
| 2,048 | 97,656 | 2.01 | 1.27 | 1.27 | 1.27 |
| 3,000 | 66,666 | 1.38 | 1.04 | 1.04 | 1.04 |
| 4,096 | 48,828 | 2.01 | 1.27 | 1.27 | 1.27 |
| 6,000 | 33,333 | 1.38 | 1.03 | 1.03 | 1.03 |
| 8,192 | 24,414 | 2.01 | 1.26 | 2.01 | 1.26 |
| 9,000 | 22,222 | 1.84 | 1.15 | 1.84 | 1.15 |
| 12,288 | 16,276 | 1.34 | 1.18 | 1.34 | 1.34 |
| 16,000 | 12,500 | 1.04 | 1.04 | 1.04 | 1.04 |
| 16,376 | 12,212 | 2.02 | 2.02 | 2.02 | 2.02 |
| 16,384 | 12,207 | 2.02 | 2.02 | 2.02 | 2.02 |

RSS KB by column (same rows): 1,500: 274,608 / 207,024 / 207,024 / 207,024; 2,048: 393,524 / 248,116 / 248,116 / 248,116; 3,000: 270,160 / 202,576 / 202,576 / 202,576; 4,096: 392,760 / 247,352 / 247,352 / 247,352; 6,000: 269,640 / 202,056 / 202,056 / 202,056; 8,192: 393,424 / 246,972 / 393,420 / 246,972; 9,000: 358,572 / 224,408 / 358,568 / 224,408; 12,288: 262,148 / 229,876 / 262,144 / 262,156; 16,000: 202,756 / 202,764 / 202,752 / 202,764; 16,376: 394,512 / 394,520 / 394,508 / 394,520; 16,384: 394,512 / 394,520 / 394,508 / 394,520.

A Bytes of n bytes takes n + 32 of the heap (24 of header, 8 of block prefix). With the
doubling classes a power of two buffer of 2, 4 or 8 KiB took the next class, 2.01 times its
payload; with a quarter step class it takes 1.26 to 1.27. Sizes between two classes (1,500,
3,000, 6,000 bytes) drop from 1.38 to 1.41 to 1.03 to 1.06. D keeps main's 16 KiB page class
for 8 KiB to 16 KiB (8,192 bytes 2.01, 9,000 bytes 1.84, 12,288 bytes 1.34); E covers 8 KiB
and 9,000 bytes (1.26, 1.15) through a 10 KiB class. 16,376 bytes and up take the 32 KiB page class in every column (2.02).

## Self build (the ADR's bar)

xo bench --ab self on linux/arm64 host 1 against main a27137842 (ab-host.sh; most rounds ran
with other jobs on the host, so wall and cpu carry its warning; max RSS is the bar's number):

| variant | rounds | wall | cpu | max RSS (KB, main to variant) | max RSS ratio |
|---|---|---|---|---|---|
| B | 3 | 0.985 | 0.981 | 4,337,788 to 4,777,320 | 1.101 worse |
| B with exact cuts (847b9b86c) | 3 | 0.992 | 0.985 | 4,346,076 to 4,498,204 | 1.035 worse |
| D | 3 | 0.993 | 0.994 | 4,372,532 to 4,247,484 | 0.971 better |
| D | 5 | 0.994 | 0.993 | 4,341,468 to 4,280,256 | 0.986 better |
| E | 5 (none loaded) | 0.981 (noise) | 0.981 (noise) | 4,307,140 to 4,409,308 | 1.024 worse |

B's first build leaked the tails of cut blocks (heapos.xo cut_for, gotchas/size-class-cut-tail-leak.md):
XO_HEAPSTATS at exit showed 815 MB of its chunks neither live nor on a free list against main's
533 MB. Cutting only exact multiples removed that (chunks 1,024 against main's 1,044), but the
peak stayed 3.5% above main while the 10, 12 and 14 KiB prefixed classes replaced the 16 KiB page
class; with 8 to 16 KiB back in the page class (D) the peak is below main. E, D with one
10 KiB prefixed class added back, peaked 2.4% above main on unloaded rounds, so D ships.

## The ADR's other rows

bench/perf/alloc, xo bench --ab on linux/arm64 host 2 against a27137842 (no round loaded):

| workload | rounds | wall | max RSS (KB) |
|---|---|---|---|
| small (short lists, classes under 1 KiB) | 3 | 1.015 worse (31.15 to 31.61 ms) | 1,124 to 1,128, within noise |
| mixed (lists of 0 to 3,000 Ints, every class) | 5 | 0.988 better (20.38 to 20.13 ms) | 10,436 to 8,396, 0.805 better |

small touches no changed class; the heap record's per class words moved from its start to its
end (offset 80 to 11,016 for the free lists), the likely cost, to be checked by moving them back.

Darwin footprint: xo bench --ab self on the Mac (darwin/arm64), caches off, 3 rounds: max RSS 1.004 within noise (4,473,024 to 4,489,728 KB), wall 1.000, cpu 0.995.

