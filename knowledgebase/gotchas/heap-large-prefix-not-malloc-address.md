# rt.heap_large's address is not the malloc'd address

Status: Observed (2026-10-14, B26 part 2; first written as Anticipated in B06, found reading xo_rt.c before the
fault record moved to Xo; not hit at run time)

Symptom (expected): a crash or heap corruption in `free` when C frees a
block that Xo allocated with `rt.heap_large`. The case here is a fault
record string that the scheduler frees with free(t->fault_msg).

Cause: `xo_heap_large(n)` mallocs n + 8 bytes, stores -n in the first
word (the large block prefix that alloc.xo reads), and returns the
address after it. `rt.heap_release` and `rt.heap_large_resize` take the
prefix's address. A pointer from heap_large handed to C's free is
therefore 8 bytes past what malloc returned.

Fix: when C will free the block, keep the malloc'd address,
`rt.heap_large(n) - 8`, and use the block from there; the prefix is not
needed outside alloc.xo (std/native/llrt/fault.xo dup). Blocks only Xo
frees can keep the usual address, freed with
`rt.heap_release(b - 8)` (testworld.xo blk_free).

Observed again 2026-10-14 (B26 part 2, entry.xo): rt.heap_release(b) of
an rt.heap_large result trapped in libmalloc (mfm_free, SIGTRAP, exit
133) on the first program start; release b - 8.

Observed again 2026-11-05 (xo-lang-4, the race stress): hserve.xo
released three rt.heap_large results (the HTTP/2 settings block
SR_H2CFG and the trailer block SK_TRL, from decisions 0433 and 0434) with
`rt.heap_release(b)` instead of `b - 8`. Nothing failed in a normal
build: free_tc read the class 8 bytes off and put a block 8 bytes off on
a free list, which silently corrupts the heap. Under `--race`, every
block comes from the C library's malloc, and TSan's malloc later
returned the misaligned chunk and stopped on its CHECK
(`sanitizer_allocator_combined.h:73`, alignment 0x8) in http_h2_config,
http_trailers and http_server_limits, 24 of 24 runs. libcmem.xo
heap_release now ends the program (status 70, "not a block's prefix")
on any address that is not 16 byte aligned, so the next one fails its
golden in every build. The full linux/arm64 golden set passes with the
check.
