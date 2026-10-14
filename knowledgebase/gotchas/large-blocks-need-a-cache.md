# Large blocks mapped from the system need a cache, as malloc has

Status: Observed (2026-10-14, work item B21)

Symptom: with the heap's large blocks (above 256 KiB) mapped with mmap
and unmapped on free (std/native/llrt/heapos.xo), bench/perf json parse
took 1.091 (arm64) and 1.106 (LLVM) times the instructions of the
malloc version; every other workload was within 1 percent.

Cause: the program makes and drops big lists again and again. malloc
keeps a large free and hands the same warm memory back; a fresh mapping
each time costs the system calls and a page fault for every page
touched.

Fix: heapos.xo keeps given back mappings in a small cache (16, 64 MiB at
most) that large blocks reuse when one fits (at most four times the
size), and resizes within a mapping in place: json parse went to 1.012
and 1.014. Watch for the same with any memory the runtime maps itself
(fiber stacks already have per worker caches, pool.xo).
