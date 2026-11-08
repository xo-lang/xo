# madvise rounds its length up over a live neighbor

Status: Anticipated (2026-11-06; no 64 KiB page Linux host to observe it)

## Symptom

On a Linux kernel with 64 KiB pages (some arm64 distributions), a
program's live 32 KiB heap block could read back as zeros after another
32 KiB block next to it was freed and scavenged.

## Cause

`madvise(addr, len, MADV_DONTNEED)` needs a page aligned `addr` and rounds
`len` up to whole pages. heapos.xo scavenge advised a free page block of
32 KiB at its own address and size, sized for darwin's 16 KiB pages: under
64 KiB pages a block on a 64 KiB boundary had its length rounded up to 64
KiB, which discards the next block's pages too, live or not (a block off a
64 KiB boundary failed with EINVAL, harmlessly). trim_list already skipped
such blocks (decision 0505's addendum).

## Fix

Every advice of a heap range uses the system's page size
(`linux.auxval(linux.AT_PAGESZ)`, at least 16 KiB): a range is advised only
from a page boundary and in whole pages, and a block the pages do not
cover exactly is left as it is (heapos.xo scavenge and advise, decision
0533). Check a new madvise caller for the same rounding.
