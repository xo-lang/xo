# 0529: Std's parse, decoded state and keys from the cache pack for warm builds

Status: Proposed (xo-lang-4, 2026-11-06; owner-approved work item, item C option b: "making std's cache reads cheap"; the target set by the step 1 breakdown below)

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

## Decision

Std, which does not change between a developer's builds or between a
suite's goldens, is kept in the cache pack in the form the build uses,
per (xo identity, std content hash, build flags), in three steps, each
measured with `xo bench --ab` on the linux/arm64 host (a warm hello and a
golden build, and one arm64 goldens suite wall) before the next:

1. Std's parse: each std file's lazy parse (its top-level declarations
   and body spans, parse_file_lazy's result) stored under the file's
   content hash and the parser's identity, read instead of lexing.
2. Std's decoded state: the front end's replayed std bodies and the
   reference counting records, kept decoded per std variant and
   referenced by the program's build, decoded only when a pass reads
   one (the rc cache's "undecoded" path extended to fe).
3. Std's keys: the cleanup, inline and function keys of std's functions
   precomputed in the pack, so a warm build hashes only the program's.

Ceiling: about 180 ms of the 357 ms of CPU per warm golden build (about
0.12 to 0.15 s of wall); the arm64 goldens about 12 s of 146 s wall; a
small program's warm build or run from about 0.3 s to about 0.15 s.

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
