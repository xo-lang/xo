# A heap profile of a large build keeps only its first 65536 samples

Status: Observed (2026-10-29, xo-lang-6)

Symptom: `XO_PROF=heap XO_PROF_HEAP_RATE=65536 xo build compiler/xo`
reported "65536 heap samples" and its alloc_space total was 5.04 GB,
all of it the lexer, parser and checker; emit, which allocates most of
the self build's 263 GB, was absent.

Cause: the sampler's table holds CAP = 65536 samples
(std/native/llrt/heapprof.xo); later samples are counted as lost (the
.txt summary says "N samples lost"), not kept, so a profile whose
program allocates more than 65536 x the rate covers only its start. The
self build at the default rate (512 KiB) is 500k samples: the profile
ends in the front end.

Fix: pick the rate from the program's allocation volume: rate >= total
bytes / 65536 (the self build: 4 MiB at least, 16 MiB to be safe,
XO_PROF_HEAP_RATE=16777216). Read the "samples lost" count in the .txt
before trusting the top list. The in-use snapshot at a label
(XO_PROF_HEAP_AT) is taken from the same table, so it has the same
limit.
