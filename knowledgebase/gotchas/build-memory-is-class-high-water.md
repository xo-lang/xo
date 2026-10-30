# A build's peak memory is the size classes' high-water, not what it allocates

Status: Observed (2026-10-29, the self build, xo-lang-6)

Symptom: a change that removed 78 GB of a self build's 263 GB of
allocations in emit (regalloc's range sorts and merges done in place)
moved the build's mapped bytes and footprint by nothing (mapped at the
end of emit 9848 against 10242 MB, peak 11425 against 11367). The
estimate "churn down, peak down" was wrong by the whole amount.

Cause: `--phase-times`' heap column is `debug.mem_mapped` (the bytes
the allocator holds from the system) and peak its high-water; the
allocator (std/native/llrt/alloc.xo, decision 0383) carves blocks per
size class and keeps freed blocks on the class's free list, cutting a
free magazine only down to a smaller class. Mapped memory therefore
tracks, per class, the most blocks ever live at once, summed over the
classes; a transient allocation that is freed and whose class is asked
for again costs nothing, however often it repeats, while a block kept
until a phase ends (a function's text string held until its part's
join; the SSA of every function held through emit) raises the
high-water of its class for good. The self build's emit added 4.4 GB of
free blocks in the 16 KiB to 256 KiB classes (free-by-class after
emit: c68 579, c69 1091, c70 1219, c71 568, c72 862 MB) with 2.3 GB
live, and the front end's freed classes were reused by the later
passes.

Fix: before estimating a memory lever, measure what is HELD per class,
not what is allocated: `XO_HEAPSTATS=1` prints the free blocks per
class at exit, and a scratch line in the driver's lap (debug.mem_mapped
plus heapos.nfree per class) prints the same after every phase; a heap
profile's inuse_space at a phase (`XO_PROF=heap`,
`XO_PROF_HEAP_AT="past phase NAME"`) gives the live side. A lever
counts only if it lowers a class's high-water: free a kept block
earlier, move it to a large block the allocator returns on free (above
the classes: a chunk joined from many small strings), or bound the
number held at once. Allocation volume (alloc_space) explains CPU time,
not memory. Use the footprint (`/usr/bin/time -l`, "peak memory
footprint") as the owner's number: mapped counts scavenged pages too
(0383 (c) returns pages of the 32 KiB and larger classes), so mapped
sat 2 GB over the footprint at the end of emit.
