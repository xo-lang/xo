# The heap record's bytes 1880 to 2135 are the thread's system call scratch, not free

Status: Observed (xo-lang-6, 2026-10-30, decision 0425)

Symptom: a built xo could not build a program: "memory budget exceeded: 18434 MB mapped" after a
few seconds (xobuild's verification), and the alloc bench died at any budget. Three new heap record
words (the page chunk's bump pointer and end, the page blocks flag) had been put at 1888, 1896 and
1904 because no allocator constant named those offsets.

Cause: obj.xo H_SCRATCH = 1880 is a 256 byte scratch area of the heap record that every system
call through the runtime writes (obj.scratch, the old xo_sys.c thread local), so the three words
were overwritten between allocations: the page bump read garbage, every page class allocation
asked heap_grow for a new 4 MiB chunk, and the budget went in seconds. grep for the offsets finds
nothing because the area is named once, by its start.

Fix: the words live past the record's last field (10952, 10960, 10968; H_SIZE grew by 24). Before
adding a heap record word, read heapos.xo's H_SIZE comment, which lists every area, and obj.xo
H_SCRATCH; a new word goes at the end, with H_SIZE grown.
