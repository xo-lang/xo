# Under XO_THP=dense a heap record's untouched bump tail is resident and nothing gives it back

Status: Observed (2026-11-17, xo-4c, linux/amd64 host; task #310)

Symptom: after a burst of small allocations on many threads and an idle
period, a native program kept about 18 MB more under the default
XO_THP=dense than with XO_THP=off, although every free list had gone to
central and the sweep gave back their units. The RSS timeline was flat
between age passes, so it was not khugepaged refilling given back
ranges (khugepaged-refills-released-pages.md), and advising each given
back page block MADV_NOHUGEPAGE changed nothing.

Cause: thp_filled advises a record's next chunk MADV_HUGEPAGE before it
faults, so the first block allocated there faults in a whole 2 MiB page.
The bump run's unused rest of that page is on no free list, so neither
the sweep (decision 0676) nor the page age pass (0533) ever sees it.
A diagnostic build that skipped the advice kept 6.6 MB, the
advice on kept 25 MB.

Fix: decision 0714's bump_age: an age pass that finds a record's bump
where the previous pass left it gives back the tail (MADV_DONTNEED,
then MADV_NOHUGEPAGE under dense). When measuring idle RSS under dense,
look for resident memory outside every free list (the bump run, words
24 and 32 of the record) before blaming the free lists.
