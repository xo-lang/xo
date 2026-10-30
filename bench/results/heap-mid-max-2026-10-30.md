# Native heap size class limit (mid_max) at start: 16 KiB on darwin

The question (xo-lang-6's peak item, 2026-10-30): the self build's peak
resident set on the Mac was 9.4 GB with 4.4 GB of freed 32 KiB to
256 KiB page blocks still on class free lists at exit (XO_HEAPSTATS).
Blocks above the size classes' limit (mid_max, XO_HEAP_MID_MAX) are
mapped from the system and unmapped when freed, so a lower limit returns
that memory at once.

## Sweep (darwin, main's xo, XO_HEAP_MID_MAX, the self build, measure window; experiment 6)

| limit | wall | peak RSS |
|---|---|---|
| 256 KiB (old default, second run; the first was cold caches, 60.75 s) | 42.30 s | 9453 MB |
| 64 KiB | 43.27 | 8668 |
| 32 KiB | 43.22 | 8522 |
| 16 KiB | 43.15 | 7473 |
| 8 KiB | 48.77 | 6473 |

The 32 KiB to 256 KiB classes are page blocks (decision 0425): mapped
directly they go back with munmap. 16 KiB: -21% peak at the same wall;
8 KiB: -32% at +15% wall.

## The default (darwin, xl103's xo, the default against XO_HEAP_MID_MAX=262136, twice each; experiment 6c)

| run | wall | peak RSS |
|---|---|---|
| 16 KiB | 42.62 s | 7481 MB |
| 256 KiB | 42.28 | 9139 |
| 16 KiB | 42.31 | 7554 |
| 256 KiB | 42.04 | 9207 |

Peak RSS -18%, wall within the pair spread.

## linux/arm64 (ab-host, the self build, 5 rounds, exclusive)

16 KiB everywhere (xl103-self, 893c08b1a's parent against it): wall 1.115
(1.084 to 1.147) worse, cpu 1.007, max_rss 1.015 within noise: Linux
returns the free lists' pages by itself (its page cache and madvise
differ from darwin's 16 KiB pages), and pays the mmap/munmap churn.
darwin only (xl103d-self, 5ab9df824 against 893c08b1a, the shipped
code): wall 1.018 (0.979 to 1.033), cpu 1.002, max_rss 0.992, all
within noise: Linux is unchanged.

Decision: 16 KiB on darwin, 256 KiB elsewhere (heapos.xo mid_default).
