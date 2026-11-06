# The large block cache refused the mapping a block's own growth made

Status: Observed 2026-11-06 (Xo Lang 2, the json10kx20 RSS item from
bench/results/release-vs-go-v0.40.0.md:612).

## Symptom

micro13's json10kx20 on linux/amd64 peaked at 44.8 MB (Go 13.9): 11.5 MB
after one round, about 2.4 MB more each round, flat from round 16. At exit
XO_HEAPSTATS showed `large cache 16 mappings (39 MB)`, all of them 4 MiB.
linux/arm64 peaked at 16 MB with one cached mapping. THP settings changed
nothing; presizing the decoder's tape and lists, and a best fit take,
changed nothing either.

## Cause

`heapos.large_resize` gives a growing large block room for four times its
new size: `large_room(n, 4 * n)` maps `4n + 16` rounded up to GRAIN
(64 KiB). `heapos.take`, which serves a request from the cache, accepted a
mapping only up to `4 * need` (need = n + 16), unrounded. For n = 1,048,496
the growth maps 4,194,304 bytes while four times need is 4,194,048: the
mapping a request's growth made could never serve that request again. Each
round's growth past 1 MiB missed, mapped a new 4 MiB block, and freed the old
one into the cache. On amd64 decision 0492 lets a recently asked for size
pass the cache's quarter bound and keeps its pages, so the dead mappings
stayed resident until the cache was full (CACHE_N, 16); on arm64 the bound
dropped them, which hid the bug there.

Found with a diagnostic build that recorded take's missed sizes in the
cache table (1,048,512 twice in three rounds) next to the cached mappings'
sizes; neither offload host has strace.

## Fix

take's bound is four times need rounded up to GRAIN, the same rounding
large_room applies. Test: std/native/llrt/heapos_test.xo (holds a large
block so the quarter bound does not hide the miss on any target).

## How to avoid

A fit test on a cache must use the same rounding as the code that makes
what it caches. When a cache's count climbs by one per iteration of a steady
loop, list what it holds and what it missed before tuning its policy.
