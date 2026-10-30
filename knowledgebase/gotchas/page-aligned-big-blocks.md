# A block's address alone does not tell a page block from a prefixed one

Status: Observed (xo-lang-6, 2026-10-30, decision 0425)

Symptom: the xo built with page blocks crashed building any program (SIGSEGV at a 4 MiB aligned
address, SIGBUS inside an unmapped 4 MiB aligned page) once the heap record words were right.

Cause: the design assumed a prefixed block's address is 8 past a multiple of 16 and so never page
aligned. A small class's stride is size + 8 with size a multiple of 16, so consecutive blocks
alternate between 8 and 0 mod 16: one small block in 2048 is page aligned, alloc.xo is_page took
it for a page block, and its "chunk table" (p & -4 MiB, the inside of some other mapping, or no
mapping) was read and written. In a sanitizer or --race build a big block from malloc can be page
aligned as well.

Fix: every chunk is 4 MiB aligned (heapos.xo map_aligned, for normal chunks too) and a page chunk's
first word is PAGE_MAGIC, which no prefix can be (a class, minus a class, or a big block's size): is_page
reads that word for a page aligned address, so a page aligned small block (whose chunk's first word is
its first block's class) takes the prefixed path. Sanitizer and --race builds turn page blocks off
(heapos.pages_on, every record's PAGES word), since their malloc'd blocks are in no chunk at all.
Any new way of handing out heap blocks must keep them inside aligned chunks or turn pages_on off.
