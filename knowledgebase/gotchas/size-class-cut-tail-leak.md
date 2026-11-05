# A cut of a larger class's block leaks its tail

Status: Observed (2026-11-05, decision 0506 stage B)

## Symptom

With four size classes a doubling between 1 KiB and 16 KiB, the self
build's max RSS rose 10% (4,337,788 to 4,777,320 KB, `--ab self` on the
linux/arm64 host) even though the new classes waste less per block.
`XO_HEAPSTATS=waste` showed the 1 to 16 KiB free lists holding less than
before (498 MB against 577 MB), yet the heap had 65 more chunks: 815 MB
of the chunks was neither live nor on a free list, against 533 MB.

## Cause

`heapos.xo cut_for` cuts the magazines of a larger class into blocks of
the class that ran out, `while off + stride <= dstride`. When the
donor's stride is not an exact multiple of the smaller stride, the rest
of the donor block is on no list and never comes back. With power of two
classes every cut was exact (decision 0383 (a)); with four a doubling
most pairs are not (a 2560 byte block cut into 1536 byte blocks leaves
1024 bytes).

## Fix

Above 1 KiB, cut_for only takes donors from the classes 4, 8, ... after
the one it fills: the same quarter of each doubling, whose blocks are
exactly 2, 4 or 8 of its own. Any change to the class sizes must keep
every donor pair cut_for can choose an exact multiple, or put the tail on
a smaller class's list.
