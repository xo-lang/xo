# The /play/ tab's memory, split (2026-11-12)

Owner question (via the lead, 2026-11-12): the playground tab, with the
REPL never opened, was reported at about 1.0 to 1.4 GB over 30 runs. Where
does that memory go, and is it a one-time cost or growth per run? This
file has measurements only, no fix.

Setup: xo 0.50.0's playground assets (version commit f764caec3's
compiler, 16.9 MB module), the site built with hugo and served locally,
headless Chrome on the development Mac (darwin/arm64), a fresh profile per
run. The download consent is set, and the default program is built and
run 30 times, each run's output checked (`[exit 0 in N ms]`). Tools:
macOS `footprint -p` and `vmmap -summary` on the tab's renderer (VM tags:
253 is PartitionAlloc, Chrome's malloc; 255 is V8; untagged mmap is wasm
linear memory), Chrome's memory-infra dumps, and a hook in each worker
(injected over CDP before the worker runs) that counts the live
`WebAssembly.Memory` objects. The driver is kept in the lead's notes
(xo-lang-2/play-mem/profile-2026-11-12.mjs).

## Where the 1.4 GB came from

It was not the tab. That figure summed the RSS of every renderer process
in the headless profile: 9 renderers, of which 8 are Chrome's own WebUI
and component extension renderers at about 100 MB each (about 900 MB),
plus the tab. RSS also counts pages that Chrome freed but the system has
not taken back yet. And the runs never accepted the download consent, so
nothing was built: a run only showed the consent box. The first version of
the release memory test (decision 0615) had the same three faults and was
fixed the same day: it now measures the tab's own footprint.

## The tab's own memory (phys_footprint, MB)

Default Chrome (Liftoff, then dynamic tier-up to TurboFan):

| point | footprint | peak so far | wasm memories (untagged) | V8 (tag 255) dirty | PartitionAlloc (tag 253) dirty (+ freed, not returned) |
|---|---|---|---|---|---|
| page ready, compiler not instantiated | 118 | 169 | 1 | 47 | 63 (+111) |
| after run 1 | 151 | 356 | 1 | 69 | 74 (+240) |
| after run 10 | 156 | 499 | 32 | 43 | 73 (+332) |
| after run 30 | 265 | 499 | 126 | 52 | 80 (+331) |
| 10 s idle after run 30 | 265 | 499 | 126 | 52 | 80 (+331) |

The release test's three runs (test-memory.mjs, same assets): footprint
peak 482, 472, 480 MB; 343, 339, 343 MB after the 30th run.

The same session with one code tier only:

| Chrome flags | footprint after run 30 | peak | live compiler instances after run 30 |
|---|---|---|---|
| default | 265 | 499 | 4 (128.8 MB of wasm memory) |
| `--js-flags=--liftoff-only` | 143 | 173 | 1 (32.2 MB) |
| `--js-flags=--no-liftoff` (TurboFan only) | 144 | 337 | 1 (32.2 MB) |

## The split

- **Compiling the 16.9 MB compiler module (one time):** the footprint
  peak. With Liftoff only the tab never passes 173 MB; TurboFan alone
  peaks at 337 MB; default Chrome, which compiles with Liftoff first and
  then recompiles hot functions with TurboFan in the background, peaks at
  480 to 499 MB, reached by run 10 and not again. The compile memory is
  PartitionAlloc (compiler zones): 240 to 332 MB of it is freed after the
  compile but stays in RSS until the system takes it back, which is why
  RSS reads 600 to 670 MB while the footprint is 265.
- **V8 (JS heaps and compiled wasm code):** 43 to 69 MB dirty in every
  mode, including the compiled compiler. The JS heaps are small: the page
  0.6 to 1.4 MB, the toolchain worker 1.8 to 3.2 MB used (8.5 MB
  reserved). These tools do not separate the wasm code from the rest of
  V8's 43 to 69 MB.
- **Wasm memory, per instance and worker:** the toolchain worker compiles
  the module once (toolchain.js:81) but makes a new instance, with its own
  32.2 MB memory, for each request (toolchain.js:267). An old instance is
  freed only by garbage collection. With default tiering, old instances
  stayed alive: 1 at run 10, 4 at run 30 (126 MB untagged), and still 4
  after 10 s idle. With either single tier, 1 at run 30. The runner worker
  (the user's program) is a new worker per run and is terminated after it,
  so its memory does not pile up.
- **The editor and page:** small. memory-infra for the tab: blink GC heap
  2 to 4.5 MB, page JS heap about 1 MB, compositor tiles 6 to 18 MB.
- **The rest:** about 10 MB (stacks, __DATA, malloc metadata).

## One-time or growth

Mostly one time: about 120 MB once the page is ready, about 150 MB after
the first build, and a peak of 480 to 500 MB during the background
TurboFan compile of the compiler module. The growth per run is the
retained compiler instances, 32 MB each: +94 MB between run 10 and run 30
with default tiering, not freed after 10 s. It did not happen with a
single tier. Whether it is bounded (a garbage collection eventually frees
them) is not measured yet.

## Experiment 1: bounded or not (100 runs, then a forced collection)

Same setup and assets (0.50.1's), 100 builds and runs, the tab's footprint
every 25 runs, then 10 s idle, then `HeapProfiler.collectGarbage` over CDP
in the page and every live worker (driver profile2-2026-11-12.mjs in the
lead's notes):

| point | footprint (MB) | wasm memories, untagged (MB) | live compiler instances (MB) |
|---|---|---|---|
| page ready | 115 | 1 | 0 |
| after run 1 | 152 | 1 | 1 |
| after run 25 | 235 | 94 | 3 (99.8) |
| after run 50 | 344 | 188 | 6 (199.5) |
| after run 75 | 257 | 95 | 3 (99.8) |
| after run 100 | 404 | 249 | 8 (266) |
| 10 s idle | 129 | 1 | 0 |
| forced collection | 123 | 1 | 0 |

Bounded, but only by V8's collector. The old instances are not leaked:
they rise and fall with V8's collections (3, 6, 3, 8), and all of them go
when V8 collects (here within 10 s of the last run; in the 30-run session
above, 4 were still alive 10 s after it). Between collections they add 32
MB each. The 404 MB after run 100 is higher than any 30-run sample
outside the compile peak. So the per-run growth is collection lag on
32 MB memories that V8 does not count as pressure soon enough. It is not
a leak.

## Next experiments

1. What delays the old instances' collection under dynamic tiering (with
   a single tier, one instance was alive after run 30): a heap snapshot
   of the toolchain worker after run 30 (HeapProfiler.takeHeapSnapshot
   on its CDP session), and retainers of the `WebAssembly.Instance`
   objects.
2. The compiled code's share of V8's 43 to 69 MB: a run with
   `--js-flags=--trace-wasm-native-heap` and the code space commits
   summed.
3. Then the fix candidates to measure, not chosen yet: reusing one
   compiler instance across requests (resetting its memory), or dropping
   each instance's references when its request ends.
