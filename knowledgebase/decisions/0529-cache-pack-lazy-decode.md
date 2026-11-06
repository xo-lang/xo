# 0529: Std's parse, decoded state and keys from the cache pack for warm builds

Status: Proposed (xo-lang-4, 2026-11-06; owner-approved work item, item C option b: "making std's cache reads cheap"; the target set by the step 1 breakdown below; continues decision 0380's open stages)

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
stage 2's 10 ms), for the suites about 9% of the arm64 goldens' build
CPU (about 12 s of 146 s wall).

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
