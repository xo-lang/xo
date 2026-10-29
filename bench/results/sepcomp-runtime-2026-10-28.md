# Separate compilation's runtime cost (decision 0388 stage 3, the default's "no runtime regression" gate)

2026-10-28, darwin/arm64 (the development Mac, 24 GB), measure class, the collatz rows at main ff712df25, the final A/B at main 792040837 (std's literals as atoms 66950ecf0, the store link's debug information 0cd8aedcb, a program importing only what its kept code calls 792040837). A = the whole program (XO_SEPCOMP=0), B = separately compiled (XO_SEPCOMP=1, std's conc0/conc1 facts and stores from `xo dev stdcache`). ~/xo-lead/notes/lang1/rtx.sh (collatz) and rtab4.sh (every row); `xo bench --ab --max-load=10`, every round usable.

## collatz: the same code, placed elsewhere

| | wall ms | instructions | cycles |
|---|---|---|---|
| whole | 450.4 | 3190.7M | 1824.7M |
| separate | 471.9 | 3194.3M | 1914.6M |
| B/A (n=20) | 1.048 (1.036 to 1.059) | 1.001 | 1.049 |

The loop's machine code is identical (otool, side by side; only addresses differ); the +3.6M instructions are startup (the empty program below). Over the 16 placements XO_ARM64_PAD=0..15 (median of 11 rounds each, cycles M):

| pad | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| whole | 1823.8 | 1809.8 | 1794.9 | 1808.8 | 1824.9 | 1799.8 | 1814.5 | 1820.4 | 1805.8 | 1805.7 | 1812.2 | 1801.0 | 1799.7 | 1818.9 | 1795.5 | 1803.1 |
| separate | 1913.3 | 1795.4 | 1786.4 | 1785.2 | 1821.9 | 1820.4 | 1781.1 | 1785.5 | 1914.3 | 1842.1 | 1827.1 | 1815.6 | 1851.9 | 1838.5 | 1785.4 | 1786.6 |

whole 1794.9 to 1824.9 (median 1807.3), separate 1781.1 to 1914.3 (median 1818.0): the distributions overlap and the separate build's best is below the whole program's; its default placement (pad 0) is one of its two bad ones. Not a regression of separate compilation (the gate's criterion for this row).

## Startup: the empty program

`fn main(os: Os) {}`, n=30. Before the fixes (ff712df25) the separately compiled program started 6 percent slower: wall 2.699 to 2.869 ms (1.063, 1.044 to 1.106), instructions retired 14.91M to 18.38M, cycles 5.6M to 8.8M. Two causes, both in the store link (xo-lang-4):

- std's literals as one atom per object, so a store link kept every std function's strings: __TEXT,__const 30.9 KB whole, 413.6 KB separate (fixed 66950ecf0: 28.9 KB).
- imports of the functions the link stripped: dyld bound 112 symbols against 54 (posix_spawn*, sockets, getaddrinfo, dlopen and the like, whose callers were gone; 7 of them weak imports libSystem lacks). A failed weak lookup costs dyld about 90K instructions (a C program with 21 of them: 10.35M to 12.25M instructions), a found libSystem symbol about 40K. On linux/arm64, with no dyld, callgrind gives the two programs' starts as 36,280 and 39,134 instructions. Fixed 792040837 (53 binds either way; it also dropped the whole program's 14 failed weak ___tsan_* imports, so every program starts faster: 15.21M to 14.32M instructions, xo-lang-4's count).

After them (792040837): wall 2.627 against 2.646 ms, B/A 1.007 (0.983 to 1.024), within noise; max_rss 2528 to 2576 KB. __TEXT 368 KB both, __TEXT,__const 30.9 and 28.9 KB, __eh_frame 14.9 and 16.1 KB, __DATA_CONST,__const 4.8 and 12.6 KB.

## Every release-vs-go workload (792040837, n=10)

| workload | whole ms | separate ms | B/A wall | 95% interval | verdict |
|---|---|---|---|---|---|
| llvm-fib | 222.8 | 223.5 | 1.003 | 0.965 to 1.04 | within noise |
| llvm-sieve | 114.8 | 109.2 | 0.951 | 0.874 to 1.053 | within noise |
| llvm-nbody | 171.0 | 168.2 | 0.984 | 0.978 to 0.987 | better |
| llvm-sort | 214.2 | 214.3 | 1.001 | 0.995 to 1.024 | within noise |
| llvm-interp | 42.38 | 41.75 | 0.985 | 0.929 to 1.107 | within noise |
| llvm-collatz | 452.7 | 474.8 | 1.049 | 1.042 to 1.053 | worse: placement (above) |
| llvm-strings | 40.23 | 40.96 | 1.018 | 0.985 to 1.047 | within noise |
| coll putget | 30.29 | 30.26 | 0.999 | 0.977 to 1.021 | within noise |
| coll get | 14.5 | 13.49 | 0.93 | 0.915 to 0.94 | better |
| coll push | 8.669 | 8.662 | 0.999 | 0.985 to 1.011 | within noise |
| coll update | 17.33 | 17.58 | 1.014 | 0.994 to 1.031 | within noise |
| coll iter | 5.749 | 5.773 | 1.004 | 0.969 to 1.025 | within noise |
| coll copymut | 8.858 | 8.916 | 1.007 | 0.99 to 1.035 | within noise |
| coll branch | 4.589 | 4.616 | 1.006 | 0.979 to 1.033 | within noise |
| conc-spawn | 20.78 | 21.23 | 1.021 | 0.988 to 1.043 | within noise |
| conc-fanout | 14.66 | 14.74 | 1.006 | 0.986 to 1.022 | within noise |
| conc-pingpong | 17.08 | 17.14 | 1.003 | 0.996 to 1.017 | within noise |
| conc-mutex | 5.986 | 5.943 | 0.993 | 0.982 to 1.024 | within noise |
| conc-rwmutex | 8.228 | 8.257 | 1.004 | 0.987 to 1.019 | within noise |
| conc-pmap | 53.84 | 54.61 | 1.014 | 0.996 to 1.028 | within noise |

max_rss is mostly 24 to 80 KB higher separately (conc-spawn +200 KB; conc-fanout 80 KB and conc-pmap 16 KB lower). The milestone's "no runtime regression" holds: every row within noise or better, and collatz's 5 percent is the same machine code at another placement, inside the spread its own placements give.
