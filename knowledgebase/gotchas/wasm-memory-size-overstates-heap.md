# The wasm memory size overstated the heap by up to 2x

Status: Observed (2026-11-10, Xo Lang 2, decision 0583)

## Symptom
The playground compiler's wasm memory read 304 MB, then 608 MB with one
more option off: always a power-of-two multiple of the same figure, and
flat whatever the program.

## Cause
compiler/wasm rt.xo's allocator grew the memory with `memory.grow` by
memory.size + 1 pages, doubling it, so `memory.buffer.byteLength` is the
heap's high water rounded up to the next doubling: anywhere from 1x to 2x
what the program used. Separately, compiler/wasm play.xo's check did not
set the checker's `lazy_parse` and `skip_tests` as `xo build` does, so it
parsed every runtime body.

## Fix
The runtime grows by a quarter of the memory (at least a page); play_check
sets lazy_parse and skip_tests. Read memory.buffer.byteLength as an upper
bound only; the free list walk (0583/jspi index-mem.html freeMB) gives
what is free. When the wasm compiler's memory differs from native's, first
compare the checker options it sets with compiler/xo main.xo's.
