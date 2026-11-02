# 0458: the heap profile folds freed samples by stack and size and recycles their slots

Status: Accepted (owner, 2026-11-02; the Lead chose variant A)

## Context

The heap profile (decision 0238 item 4, `std/native/llrt/heapprof.xo`) kept every sample in a table of
CAP slots that was never recycled, so a program that allocated more than CAP times the rate in bytes lost
the rest ("samples lost"; 2,000 TLS handshakes filled the old 65,536, and the cap went to 262,144 on
2026-11-01). A long running server runs past any fixed cap, and its alloc_space and alloc_objects then
undercount, which misleads.

## Decision

- A sampled block that is freed is folded into an aggregate keyed by its stack and its size: one entry
  holds the stack, the size and how many freed samples it stands for (16,384 entries, 8.5 MB of address
  space, pages touched as entries are made; an insert is a compare and swap on the entry's state, then a
  counter add). Samples of one stack and size scale the same way, so the count times the one scale is
  exactly what each of them would have added: alloc_space and alloc_objects stay exact for any run length.
- The freed sample's slot goes on a stack of free slots (a Treiber stack, a tag in the bits above 20 against
  ABA, links in the slot's size word) and the sampler takes from it before it takes a new slot: a sample is
  lost only when the table is full of live blocks or the aggregate is full.
- The writer (`prof.xo` heap_section) emits section type 18 (PS_HEAP2): each record is the stack, the size,
  freed (0 live, 1 freed, 2 untracked) and the number of samples it stands for. A live slot is count 1; an
  aggregate entry is freed with its count. `xo prof` reads both type 17 (count 1 for every record) and 18.
- A block that is resized keeps its slot (heapprof.xo resized): only a real free folds.

## Consequences

- Files written by this version are not read by an older `xo prof` (it ignores the new section type); files
  from older versions read as before (TestProfOldHeapRaw, testdata/prof/heap-v17.xoraw).
- inuse values come from the live slots only, as before.
- A free of a sampled block costs a hash of its up to 64 pcs and two atomics (once per rate bytes allocated,
  so rare).
- `TestProfHeapRecycle`: 400,000 short lived lists at rate 1 (against 262,144 slots) lose no sample and
  count 400,000 objects.

## Alternatives

- Recycle a freed slot and report "samples recycled N", leaving alloc_space and alloc_objects undercounted
  by them (one file, no format change): rejected; the undercount is the failure being fixed.
- A bigger cap: memory and address space grow with the run and still end.
