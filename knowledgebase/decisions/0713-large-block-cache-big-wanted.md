# 0713: the large block cache keeps a big block a recent request could use past its 64 MiB bound

Status: Accepted (owner decision #150, 2026-11-18, answering ask #131;
task #302). Refines decisions 0448 and 0492 (std/native/llrt/heapos.xo
keep and keeps). a437fd4a2 (task #271, 2026-11-18) had already made the
same rule for x86-64, bounded by the program's peak; this extends it to
the other targets.

## Context

Task #302's Proven perf evidence on the linux/amd64 host (2026-11-17)
timed bench/perf/repeat at 3.1 to 3.2 of Go: byte1 1224 ms against 380,
byte3 1086 against 344, byte100 1116 against 360. Each workload makes a
Str and a Bytes of 150 to 160 MB and frees them, ten times. On the Mac
(byte1, 2 runs): Xo 0.29 s (0.10 user, 0.18 sys) with 205,239 page
reclaims, Go 0.12 s (0.04 sys) with 21,210; the peak footprint is the
same (338 against 342 MB). 20 blocks of 160 MiB over 16 KiB pages is
204,800 pages: every block was mapped and faulted in afresh.

The large block cache (0448, 0492) keeps a freed mapping only while it
holds at most CACHE_N (16) mappings and CACHE_MAX (64 MiB) bytes, and,
on arm64, a quarter of the bytes mapped. A 160 MiB block passes CACHE_MAX
alone, so it always went back to the system, even when the program asked
for that size a moment before and asks again a moment after. Go's heap
keeps freed spans and its scavenger returns them in the background.

## Decision

A freed block over 16 MiB (CACHE_MAX / 4) that one of the last 16 sizes
asked of the cache could use (0492's `wanted`: need <= size <= 4 need)
stays in the cache past CACHE_MAX and the quarter bound, on every target
(x86-64 already kept any wanted size this way, a437fd4a2), while the
cache's bytes stay at most the program's peak use (membudget.peak, as
a437fd4a2 bounds it); CACHE_N still bounds the mappings. The owner's
condition (#150): such a block is unmapped by decision 0714's idle age
pass like any other cached mapping (cache_age: two passes with it
unused). Its pages are not advised away (0492:
a wanted block keeps them), so the next request of that size reuses them.

## Consequences

- bench/perf/repeat/byte1 on the Mac: 0.29 s to 0.08 s (Go 0.12 s), page
  reclaims 205,239 to 41,383, peak footprint unchanged (338 MB).
- xo's own build of compiler/xo on the Mac, old then new xo, 2 rounds
  each: wall 30.3 and 31.7 s against 31.8 and 31.8 s; max RSS 4.38 and
  4.63 GB against 4.23 and 4.32 GB; peak footprint 3.71 and 3.62 GB
  against 3.66 and 3.66 GB: within noise.
- The trade: a program that frees a big block and asks for that size
  again keeps the block's memory resident between the two (at most
  CACHE_N blocks); one that frees it for good holds it until a later
  free of another size pushes it out of the ring's fit, or to exit. Its
  max RSS does not rise (the block was already mapped), but its RSS
  after the last use stays at that peak only until the idle age pass
  (0714) finds it unused twice and unmaps it.
- Guard: internal/arm64 TestBigBlockReuse (48 MiB Str and Bytes made and
  freed 10 times: 62,337 minor faults on the Mac before, 13,161 after,
  ceiling 4 x 96 MiB of pages).

## Alternatives

- Raise CACHE_MAX: every freed block of a size up to the new bound would
  be kept, wanted or not, holding memory a one-off free never uses again.
- madvise the block's pages away and keep the mapping: saves the mmap and
  munmap but not the page faults, which are the cost here.
- A time bound (keep a big block for N ms after its free): needs a clock
  read on the free path; 0714's age pass gives the same bound with no
  new clock.
