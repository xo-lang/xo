# 0529: Std's parse, decoded state and keys from the cache pack for warm builds

Status: Accepted (owner, 2026-11-06). Proposed (xo-lang-4, 2026-11-06; owner-approved work item, item C option b: "making std's cache reads cheap"; the target set by the step 1 breakdown below; continues decision 0380's open stages)

## Context

Item C (bench/results/suite-fixed-cost-2026-11-06.md) measured the fixed
cost of a warm `xo build`: 0.22 to 0.36 s for a small program, nearly
all of it std, paid by every golden of every suite (arm64 648 builds,
llvm 596, std 353, `go test -short` 507) and by every developer build
of a small program. It recommended this instead of decision 0283 stage
2 (a resident compiler), whose ceiling for the suites was under 3% of a
tier.

The step 1 breakdown (host 1, ac2664c08, 2026-11-06): a warm build of
the golden json_decode_fields, XO_BIN_CACHE=off (so the whole-build run
cache of decision 0360 is off), wall 0.29 to 0.36 s, 357 ms of CPU per
build over 20 profiled builds:

| what | CPU ms per build |
|---|---:|
| lexing and body-skipping every source, std's 113 files included (parse_file_lazy; "cache parse 113/113 reused" is the build target's parse list, not the tokens) | 58 |
| the check (check_program) | 42 |
| the front end's replay (fe_cached_build; decode_func 23) | 42 |
| pack reads and LZ4 (cache_get_packed) | 30 |
| reference counting's cache (rc_cached) | 22 |
| Str map hashing | 22 |
| SHA-256 of cache keys | 21 |

The bytes read (about 24 MB) are not the cost; parsing std again,
decoding and replaying std's records and computing their keys are.

## Relation to decision 0380

Decision 0380 (accepted 2026-10-26) already covers the parse part: its
stage 1 (7b32ec87e) is the lossless parse codec (compiler/parser
astcodec.xo), its stage 2 (std's parses from the back-end cache,
XO_IFC_CACHE=1, refs/recover/lang1-0380-stage2) measured about 10 ms of
one edit's wall and was not landed, because the parse runs on prefetch
tasks off the critical path and a decoded tree allocates as much as a
parsed one; its stage 3 (declaring std from entries) is open. The 58 ms
above is that same parse: CPU, mostly not one build's wall.

What 0380 did not measure is CPU under parallel load. The suites build
their goldens concurrently and are CPU bound (arm64 goldens: 2.07 s a
build in the suite against 0.22 to 0.36 s alone), so CPU per build is
what sets their wall, and the parse's 58 ms of CPU counts there in full.

## The suites take separate compilation instead (2026-11-06)

Item C's fix (c) (bench/results/suite-fixed-cost-2026-11-06.md): with
std's two variants of decision 0388 prebuilt by the test harness, the
arm64 goldens took 50 to 80 s of wall and 335 to 362 s of CPU against
131 to 177 s and 1360 to 1369 s whole program; 0380 stage 2 on the
same suite saved 5 to 10% of CPU. So the suites build separately, and
this decision is for the builds that stay whole program: xo building
itself, `--release` builds, cc links, the whole-program sample and full
tier that keep the goldens' whole path covered, and a developer's
first builds before the background build has made the variants. Each
step below is judged on those builds, not on the suites.

## Decision

Build on 0380 rather than beside it; each step measured on the
linux/arm64 host by both a single warm golden build (`xo bench --ab`,
wall and CPU) and one arm64 goldens suite wall (parallel, CPU bound),
before the next:

1. 0380 stage 2 rebased onto main (std's lazy parses from the cache),
   judged by the suite wall and CPU, not a single build's wall.
2. 0380 stage 3: std modules declared from entries (the checker's
   declarations serialized, types by stable names), the check's std
   share (about 30 of 42 ms).
3. New here: std's front end replay and reference counting records kept
   decoded per std variant and decoded only when read (the rc cache's
   "undecoded" path extended to the front end), and std's cleanup,
   inline and function keys precomputed in the pack.

Ceiling: about 180 ms of the 357 ms of CPU per warm golden build; for a
single build's wall less (the parse is off the critical path: 0380
stage 2's 10 ms). The suites are no longer the target (above).

## Results (2026-11-06)

- Step 1 (std's parses from the cache's ast/ entries, XO_IFC_CACHE on by
  default): the arm64 goldens suite's CPU 110.3 to 105.1 s (-5%) on the
  linux/arm64 host; xo's self build within noise. Small whole builds
  with a back-end cache per side: json_decode_fields wall 0.924, CPU 0.840; http_server_request wall 0.963, CPU 0.885 (B/A, linux/arm64 host, the same in both run positions: wall 0.942/0.915 and 0.966/0.967).
- Step 2 (std declared from decision 0388's records for whole-program
  builds too) is held. load_std_record makes every module of the
  record's closure (modules, files, check order) up front in the order
  of the program that wrote the record, so a whole build's bytes
  depended on which program wrote it: TestFrontCacheAcrossPrograms
  (three orders) and TestChainKeysSharedCache failed with it and pass
  with step 1 alone. It stays on for separately compiled builds, where
  std's code comes from its object. To come back for whole builds it
  needs the record's modules made as the program's imports reach them
  (the source check's order), and a gain of its own: xo's self build
  measured 0.995 wall with it.
- Step 1's first gate failed separately compiled tests of the full
  internal/arm64 run; they were the suite's std builds killed for
  memory (`xo dev stdobj exit 137`, about twenty parallel std builds
  past a 22 GB cgroup), which main failed the same way, and a cold std build's peak was
  1.39 GB with and without step 1. The gate also found that the AST
  codec dropped File.ranges (gotchas/ast-codec-dropped-decl-ranges.md),
  fixed in codec v8 with step 1.
- Measuring note: the first small-build A/Bs shared one back-end cache
  between the two xos, which slowed the second build of each round by
  about 25% (gotchas/bench-run-position-effect.md); the figures above
  use a cache per side.

## Consequences

- A cache format addition (new pack kinds); entries are keyed by
  content, so a wrong entry cannot be served for changed sources. Each
  step's gate runs the cache packages' own xo tests, an identity check
  (a binary built with the step on equals one built with it off), and
  XO_BACKEND_CACHE=verify on the goldens it touches.
- Each step has an off switch (XO_STD_PACK=off, by step) for bisecting.
- The program's own entries stay as they are: the step 1 numbers put the
  program's share small for the goldens; an edit loop's share is
  measured after step 2.

## Alternatives

- Decision 0283 stage 2 (a resident compiler): the same per-build
  saving for the suites, plus a process every test driver must manage;
  under 3% of a tier (item C).
- Mapping the pack (mmap) without changing what is stored: saves the
  read copy only, about 10 to 20 ms of 357.
- Building many goldens in one xo process: `xo build A B` is one
  program; a multi-program mode in xo and the test drivers, for the same
  ceiling.
