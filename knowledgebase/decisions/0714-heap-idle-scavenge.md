# 0714: The idle age pass gives back every record's free lists, spares and bump tail

Status: Accepted (lead decision #129, 2026-11-18; task #310, xo-4c). Supersedes the spare rule of 0704 (its second look at an unchanged spare).

## Context

Decision 0676's sweep gives back whole 4 KiB units of the prefixed size
classes on Linux, and 0704 added a spare magazine that no free or refill
replaced between two age passes. The stress runs of std/sse, amqptest,
grpcreflect and otlp (decision #125) still grew 42% to 84% from minute 5
to the end. TestRuntimeHeapIdleLists (sixteen tasks at once make and free
104 MB of small blocks, then the program idles) kept 67 to 83 MB after
idle. Three causes:

- The sweep reads only the central lists and the claiming record. A unit
  whose blocks other threads freed sat on their records' free lists and
  never went back.
- An idle worker that marks nothing runs one age pass and sleeps, so
  0704's second look at its spare never came.
- Under XO_THP=dense (the Linux default) thp_filled advises a record's
  next chunk MADV_HUGEPAGE before it faults, so the first block there
  makes a whole 2 MiB page resident. The bump run's unused rest is on no
  free list, so neither the sweep nor the page age pass sees it (18 MB
  of the remainder; 7 MB with the advice off).

## Decision

At every age pass, for each heap record that is not a region record:

1. Each prefixed class's spare magazine and free list go to that class's
   central list (alloc.spare_to_central, alloc.list_to_central), so the
   same period's sweep sees all of their blocks. A busy record takes them
   back at its next refill.
2. The bump run's unused tail (cur rounded up to a page, to end rounded
   down) is noted at one pass. If the next pass finds the bump where it
   was, the tail is given back with MADV_DONTNEED, plus MADV_NOHUGEPAGE
   under dense so khugepaged does not fill it again. H_IDLE's word 0
   holds the bump seen, with its low bit set once given. thp_filled
   advises the chunk again once it is used up.
3. A large block cache mapping that two passes found unused is unmapped
   (cache_age, CACHE_SEEN; take clears the mark).

No new variable: the behavior is always on.

## Consequences

- TestRuntimeHeapIdleLists: 67 to 83 MB kept after idle before, about
  7 MB after (the guard fails above 8 MiB).
- The stress A/B numbers are in task #310's notes
  (~/xo-lead/notes/rss-0704/) and in the commit.
- A worker idle for one period pays one lock per class with a list,
  about 76 lock pairs per record per 30 s.
- The bump tail's madvise can split the chunk's mapping into at most
  three VMAs, once per record per chunk.

## Alternatives

- The sweep walks every record's free lists. It would have to take every
  record's lock, and the record's owner holds no lock on its own list.
- Advising each given back page block MADV_NOHUGEPAGE: measured, no
  change; the residue was in the bump tail, not the page blocks.
- XO_THP=off as the default: kept 4.6 MB, but loses dense's huge page
  speedups (decision 0456).
