# 0351: Warm builds without per-directory loose reads

Status: Proposed (Xo Lang 2, 2026-10-23). Measurements and the design
follow; the lead approved the investigation, the owner decides.

## Context

A warm `xo build` of a one-line program (darwin/arm64, xo at 923aed0a4,
a warm backend cache; ~/xo-lead/notes/xo-publish/warm-build-profile.md):

- in a working directory whose packs exist: real 0.30 s, user 0.65 s,
  sys 0.13 s;
- in a new working directory, same cache: real 0.38 to 0.55 s, user
  0.5 s, sys 0.84 to 0.92 s on each of its first three builds; XO_PROF
  puts 65 to 70% of the samples in `open`.

Packs are keyed by the working directory plus the command's words
(bcache.xo pack_dir, so one program's segments do not hold another's).
Until a directory's packs form, every entry the build reads (parse, fe,
borrow, rc, inl, sum, fn, obj: about 6,000 files for hello, almost all
of them std and runtime functions every program shares) is a loose file
opened in parallel. Every caching layer hits on such a build; the time
is the opens.

Two smaller findings on the way: every warm build in any directory also
tried 4,146 front end keys loose that are never stored (one key per
checked body, the front end stores only the bodies it builds); that was
fixed on its own (a745a66fe, a negative list in the formed pack, warm
hello CPU 0.929). And the check phase of a warm hello build (54 ms CPU)
is half reparsing all 191 files and a third declaring and resolving
their signatures; std bodies' check is about 5 ms, so caching std
modules' checks was dropped.

## Decision

Proposed, in stages, each with its own `xo bench --ab`:

1. **A shared pack per kind and target** (compiler/xo bcache.xo
   shared_pack_dir, shared_read, shared_store; behind `XO_SHARED_PACK=on`
   until the owner decides the default). Reads go: the build target's
   own pack, then the shared pack, then loose files. A build that read
   64 or more entries loose appends them to the shared pack as a
   segment; past 4 segments, or when the pack holds more than twice
   what the build used of it, the build rewrites it as one segment of
   what it used (the entries it read from it and its loose ones). It
   therefore settles on what every program uses (std and the runtime)
   without knowing which modules those are; one program's own entries
   live in its own pack. Parts (one program's split) are never shared.
   The prune walks `packs/shared` as one shard, and reads mark segments
   used, as for the other packs.

   --ab, one xo, A = `XO_SHARED_PACK=off`, B = on, 20 rounds, warm caches
   (hello's), 2026-10-23:

   | case | wall B/A (95%) | CPU B/A (95%) |
   |---|---|---|
   | first build of a new program, new directory | 0.831 (0.787-0.866) | 0.509 (0.489-0.59) |
   | warm hello, new directory | 0.857 (0.83-0.888) | 0.593 (0.55-0.653) |
   | warm hello, same directory | 0.994 (0.973-1.032), noise | 1.006 (0.988-1.024), noise |

   The shared pack held 16 MB after these builds.

2. **A cross-build parse and declaration cache**: each file's parse (or
   each module's declarations) keyed by its bytes and the parser's
   version, read from a pack, so an unchanged std file is neither parsed
   nor declared again (half of a warm hello's check). Its own --ab.

3. **A whole-build key** (Go's action IDs): the hash of the checked
   modules' keys, the flags and the target maps to the linked binary;
   on a match the build copies it and skips the front end, the passes,
   code generation and the link. Its own --ab.

## Consequences

- Stage 1 changes the cache layout (a new `packs/shared` tree) but not
  any build's output (TestSharedPack: equal to a cache-off build).
- Turning it on by default is the owner's call (a default change);
  until then it costs nothing.
- A new directory still pays failed fe lookups (9,520 for hello): the
  negative list exists only once a directory's pack forms. A shared
  negative list is a follow-up.

## Alternatives

- Key the per-directory pack by content instead of the working
  directory: one program's pack would then serve every directory, but
  the decision of 2026-10-22 keyed it by directory so one program's
  segments never hold another's; a shared pack keeps that and shares
  only what every program reads.
- Mark std entries explicitly (by module path) for the shared pack:
  needs every cache kind to know its entries' modules; the compaction
  rule gets the same set without that.
- Fewer, larger loose files (sharding by key prefix): still one open per
  entry at the first build of each directory.
