# Draft: Hunting a slow memory creep in Xo's heap

Status: Draft (xo-4c, 2026-11-19, task #431, for the owner; not
published). Figures come from the run notes in the commit message of
207a53636, `knowledgebase/gotchas/dense-thp-pool-run-drops-half.md`,
and the stress outputs behind them, and the follow up fix 0d8e474b2.

---

Every standard library module in Xo has to pass a stress run before it
is called Proven. For a long running server module the stress run
checks two things: every answer is right, and memory does not keep
growing. The second check is simple. The program samples its resident
memory (RSS, the pages the process actually holds in RAM) every minute,
takes the median of a few minutes in the middle as a mark, and compares
the median of the last three minutes against it. On a 60 minute run,
more than 5% growth past the mark fails.

This is the story of one module that kept failing that check, and of
the two small bugs in the runtime's heap that turned out to be behind
it. Neither bug lost memory in the usual sense. Nothing leaked, the
live data was flat, and the leak checker found zero live allocations
at exit. The process just slowly held more pages than it needed.

## The symptom

`std/grpcreflect` answers gRPC reflection requests. Its stress run
sends about 130 to 160 million requests from 64 tasks over an hour.
On one of our linux/arm64 test hosts, its 60 minute runs grew 5%, 10%
and 3% in three tries: a failure one time in three, and passes that
sat close to the line. A second host passed at 2%. Then `std/ui`'s
stress run showed the same pattern on two backends (10% on LLVM, 20%
on arm64 in one run, 1% in the next). A third module with the same
shape meant the cause was in the runtime, not in any one module.

A flaky growth check is easy to "fix" by raising the bound. We did
not, for the same reason we never re-record a golden file to get a
change merged: the check was saying something real, and we did not
know what yet.

## First, rule out the obvious

The first questions were the cheap ones.

- **Is live data growing?** `XO_HEAPSTATS=1` prints the heap's live
  block count. It stayed flat at about 9,000 blocks all run. Not a leak
  in the program.
- **Is it the large allocations?** A run that recorded every mapping
  over 4 MiB showed no large blocks at all.
- **Is it fiber stacks?** Cached stacks keep their deepest call's pages
  resident, which looked like a good suspect. Trimming cached stacks in
  a prototype changed nothing that mattered.

Then `/proc/PID/smaps`, the kernel's per mapping view of a process,
gave the first real lead: the growth was inside the heap's own 4 MiB
chunks. Mappings of 1 to 4 MiB grew in number and in RSS while whole
4 MiB mappings shrank. Chunks were being split into smaller mappings,
which is what `madvise` calls do when they cover part of a range.

## Turning on the per minute view

`XO_HEAPSTATS=pages` breaks the heap down at exit: how many chunks,
how much of them is on free lists, and how much of that is whole free
4 KiB pages. The debug endpoint `/debug/sched` gave the same heap
lines every minute while the server ran. Together they showed the
shape over time instead of a single snapshot:

- in passing runs, the chunk count stayed at 16 from minute 1;
- in failing runs, the heap mapped new chunks late in the run,
  reaching 20 to 22, and RSS followed each one up.

So the question narrowed from "why does memory grow" to "why does the
heap map more chunks when the live data is flat".

## One path off at a time

Xo's heap has a Linux specific mode for transparent huge pages,
`XO_THP=dense`, which is the default there. In dense mode a new chunk
is reserved in two halves: the first 2 MiB is used at once, and the
second half is opened later, when the first fills, and advised
`MADV_HUGEPAGE` so the kernel can back it with one 2 MiB page. When a
chunk fills, the heap also advises the filled chunk as a huge page
range. That is three separate behaviors, any of which could map or
hold pages differently from the plain mode.

Each experiment below was a diagnostic commit that was never pushed:
it turned one behavior off, ran the same 60 minute stress twice on the
same host, and compared growth and chunk counts. Thirty minute runs
were not enough; dense and the variants all passed at 30 minutes and
only separated in the second half hour.

| Variant | Growth (60 min, two runs) | Chunks at end |
|---|---|---|
| Dense, as shipped | 5%, 10% FAIL, 3% (three runs) | 18 to 21 |
| Dense, no huge page advice on reused runs | 7%, 7%, both FAIL | 18, 22 |
| THP off entirely | 4%, 6% | 16, flat |
| Dense with the halves layout off | 4%, 4% | 16, flat |
| Dense halves kept, no `MADV_HUGEPAGE` at all | 6%, 5% | |

The second row disproved our first hypothesis: the advice on reused
pool runs was not the cause. The third row was the control: with
huge pages off, the chunk count never moved, and the small growth
left was the arena filling toward its mapped size and leveling off.
The fourth row put the cause on the halves path. The last row said
the advice explained part of the growth but not all of it.

With the field that small, reading the code found it.

## Two bugs on one path

When the heap's current chunk fills, `heap_grow` tries, in order:
take a run of free pages from the pool, or grow the chunk in place,
or map a new chunk.

**Bug 1: the pool came first.** In dense mode, "grow the chunk in
place" means opening the reserved second half. But `heap_grow` asked
the pool first, and taking a pooled run marked the chunk as having no
reservation. The second half was dropped without ever being used, and
the next time the run filled, the heap mapped a whole new chunk. With
halves on, the pool handed out 240 to 369 runs per hour; with halves
off, none. Each one could cost a chunk.

**Bug 2: the wrong range got huge pages.** When a new chunk is
mapped, the heap advises the chunk that just filled as `MADV_HUGEPAGE`,
using the end of the bump range as that chunk's end. After a pooled
run, though, the bump range's end is the end of the run, which sits
inside some other chunk. The heap advised the 4 MiB before it, a range
that held pages the sweeper had just given back to the kernel. The
kernel's background collapser, khugepaged, then filled those pages in
again to make whole huge pages. That is the slow creep: pages returned,
then quietly made resident again.

## The fix and the tests

The fix is three lines of logic in `std/native/llrt/heapos.xo`:

- `pool_take` returns false while the chunk can still grow in place,
  so the reserved half is opened first;
- a pooled run marks the record with its own value (`BLIM_RUN`),
  distinct from "no reservation";
- `chunk()` passes no filled end to the huge page advice after a
  pooled run, because the run's end is not a chunk's.

Each bug has a test in `heapos_test.xo`, and both tests failed before
the fix on linux/arm64:

- a record with its second half reserved is given a pooled run, and
  the test expects `pool_take` to decline and the next bump to land in
  the second half;
- a chunk mapped after a pooled run is checked in `/proc/self/smaps`:
  no mapping in the 4 MiB before the run's end may carry the `hg`
  (huge page) flag.

The second test reads the kernel's own view of the process rather than
the heap's bookkeeping, so it checks the effect that mattered, not just
the variable we changed.

## The fix made it worse

The first guard after the fix did not pass. grpcreflect grew 6% on
one host and 14% on another, where it had grown 2% before. And an
older test, `TestRuntimeHeapIdleTHP`, began to fail on linux/amd64. It
allocates about 64 MB, frees it, then sits idle for 150 seconds while
it watches its own RSS. With the fix, RSS ended 9.4 MB above its
starting point instead of falling back to it, and `AnonHugePages` rose
from 0 to 6 MB while the program did nothing at all.

The fix was doing what it said: the reserved second halves were now
opened before the pool was asked. But each second half is advised
`MADV_HUGEPAGE` when it opens, so far more of the heap now lived in
memory the kernel was invited to collapse. That exposed two places
that gave pages back without taking the invitation away:

- the idle pass, which returns free page blocks with `MADV_DONTNEED`
  and nothing else. A hole in a huge page range is exactly what
  khugepaged fills. The scavenger already paired its `MADV_DONTNEED`
  with `MADV_NOHUGEPAGE` on the chunk; this path had never needed to;
- the advice on a used up chunk, which turned huge pages back on over
  runs the sweeper had just turned them off for.

Each change was measured alone on the same amd64 host, as KiB over the
start after 150 s idle (the test allows 8,192):

| Variant | KiB over |
|---|---|
| The first fix, as pushed | 9,440 |
| Plus the used chunk advice change only | 9,468 |
| Plus the idle pass change only | 3,836 |
| Plus both | 240 |
| The first fix reversed | 3,804 |

The used chunk change alone did nothing measurable here, but it is
a real bug with its own failing test, and with both changes the
result is better than without the first fix at all. The rule both
changes follow: every path that gives pages back under dense mode also
turns huge pages off for them, and nothing turns them on again until
the memory is reused.

## After the fix

With both follow up changes in, `TestRuntimeHeapIdleTHP` passes on
linux/arm64 too, on a host with huge pages set to `always`: 3,964 KiB
over the start on the native backend and 3,808 KiB on LLVM, against
240 KiB on amd64.

grpcreflect's 60 minute stress, run alone on the host that had grown
14%, grew 3% and 3%. The tree from before the first fix, run the same
way, grew 3% and 2%. One more lesson came from that pair: an earlier
attempt ran both trees on the host at once, and both failed (34% and
25%) with less than a third of the requests. Two 64 task stress runs
sharing 18 cores measure each other, not the heap.

## What we would do again

- **Keep the bound.** The growth check was right, and a looser bound
  would have shipped both bugs.
- **Run long enough to separate.** Thirty minutes passed everything.
  The failures only showed in the second half hour, so every later
  experiment used 60 minutes, two runs each.
- **One path off at a time, with a control.** The THP off control
  showed what normal growth looked like; without it, "4%" would not
  have meant anything.
- **Disprove out loud.** Two hypotheses (fiber stacks, pool re-advise)
  were tested and dropped. Writing each one down with its numbers kept
  the next experiment honest.
- **Test the effect, not the variable.** Reading `smaps` in a unit
  test is unusual, but it is what khugepaged sees.
- **Expect a fix to move load onto other paths.** Opening the halves
  first was right, and it sent more memory through two give back
  paths that had been safe only because they were rarely used. An idle
  test caught it within a day; measuring each follow up change alone
  showed which one mattered.
- **Compare on a quiet host.** A pair that shares a machine is not a
  pair.

The full run notes, including every experiment above with its run
names, are in the task record for #355.
