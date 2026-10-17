# A fault benchmark gets much slower when unrelated code moves

Status: Observed (2026-10-16 and 2026-10-17, macOS arm64, bench/perf
fault isolate)

Symptom: bench/perf/fault's isolate workload ran 1.14x (2026-10-16) and
1.42x (2026-10-17) the instructions after changes that did not touch the
fault path (the second: arm64's inline string push). Linux was not
affected, and the extra cost grew from about 1,000 faults on (a single
fault cost the same).

Cause: std/native/llrt/unwind.xo lookup_frame caches the frames it reads
(begin, end, LSDA, encoding, the CFA and register rules) in a set
associative cache keyed by pc. A fault walks about a dozen frames; when
more of their pcs map to one set than it has ways, they evict each other
on every fault and each lookup reads the unwind tables again. Which pcs
share a set depends only on the code's layout, so any change that moves
code can trip it. It was direct mapped until 2026-10-16 (two pcs
collided), then two way (three collided on 2026-10-17).

Fix (2026-10-17): the set is a multiplicative (Fibonacci) hash of the
pc, `((pc >> 2) * 0x9E3779B97F4A7C15) >> 57`, over 128 sets of two ways
(FC_ENTRIES 256 of FC_BYTES 512: 128 KiB of the thread's scratch
mapping, its pages touched only as sets fill on Linux and macOS; Windows
commits the mapping with VirtualAlloc, so 96 KiB more per thread
there), so a code move cannot
line the hot frames up by their low bits. With about a dozen hot frames
a walk, three meet in one set in about 1% of layouts.

Measured at main d28569ea (fault isolate, defer, owned; macOS
instructions retired, Linux callgrind), against the 32 set two way
index: macOS +0.5%, +0.5%, +0.4%; Linux +0.2%, +0.3%, +0.4% (the
multiply). Four ways (16 sets) cost +1.1% on both, and four ways with
the hash +1.4%. Neither layout thrashed on that commit; the earlier
layouts did (isolate 1.42x on macOS, and on Linux at 21ea4e03 269M
instructions against 160M).

If it comes back, count the misses a fault takes; more sets cost
nothing per lookup, more ways cost a probe each.
