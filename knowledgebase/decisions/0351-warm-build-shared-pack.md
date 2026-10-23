# 0351: Warm builds without per-directory loose reads

Status: Accepted (owner, 2026-10-23): stage 1, the shared pack, is on by
default (XO_SHARED_PACK=off turns it off). Stage 2 (lazy function bodies)
is in progress; stage 3 (a whole-build key) is proposed.

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
   until the owner's decision of 2026-10-23 made it the default). Reads go: the build target's
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

2. **Lazy function bodies** (approved by the lead 2026-10-23 in place of
   an on-disk parse cache: no AST serializer exists, and decoding would
   allocate the same nodes): a warm hello build allocates 1.33 GB, 538 MB
   of it parsing, mostly std and runtime bodies it takes from the front
   end cache. The parser's lazy mode (parse_file_lazy) skips each
   function body by its braces at the token level and records its range
   (FnDecl.lazy); parse_body parses it with the full parse's spans
   (lexer.lex_from). Stage (a), the parser alone, is in: the lazy parse
   with every body forced dumps exactly as the full parse for all 1,785
   repository files that parse cleanly (35,374 bodies;
   internal/selfhost TestParserLazyBodies, `xo dev dump ast --lazy`).
   Stage (b) chooses the mode per file (lazy only for the toolchain's
   own std and runtime and for modules whose check cache entry hits),
   forces bodies through one checker entry point, and gives an unforced
   body a sentinel that faults when read; its gate builds compiler/xo and
   every golden with XO_LAZY_PARSE=on, the fixed point and identity. Its
   own --ab (design: ~/xo-lead/notes/xo-publish/0351/lazy-body-parse.md).
   Stage (b) landed opt in (XO_LAZY_PARSE=on, 2026-10-23; lazy for std
   and runtime files in build and run): 33 of 33 programs identical to
   a full parse, compiler/xo included; a small program's warm build
   0.589x peak memory and 0.886x CPU, wall within noise, compiler/xo
   unchanged (bench/results/lazy-parse-2026-10-23.md). On by default
   since 2026-10-23 (owner), after the touched packages' full suites
   passed with it on (compiler/parser and compiler/check xo tests,
   cmd/xo, internal/arm64 TestGoldenRun); XO_LAZY_PARSE=off is the
   escape hatch.

3. **A whole-build key** (Go's action IDs): the hash of the checked
   modules' keys, the flags and the target maps to the linked binary;
   on a match the build copies it and skips the front end, the passes,
   code generation and the link. Its own --ab.

## Addendum (2026-10-23): `xo check` uses the module check cache

Lead-approved (b08 follow-up). `xo check` checks a program module
unchanged since its last `xo check` (its key: the files, the interfaces
it imports, xo's cache ID, the std path) from its entry instead: the
module's diagnostics, warnings included, are replayed, so the output is
a full check's. The entries are `xo check`'s own (flags `xo check`),
apart from a build's, whose entries also say what the front end cache
replays. A plain check also checks the native runtime's bodies lazily,
as a build does (decision 0247): no SSA builder runs, so they are not
checked; a named directory inside std keeps the eager check. A checked
body that reads a skipped module's inferred effects makes the check run
again without the cache. `--support`, `--backend=go`, an overlay (the
editor) and XO_CHK_CACHE=off, XO_FE_CACHE=off or XO_BACKEND_CACHE=off
check everything; XO_CHECK_EAGER=1 checks the runtime's bodies too (cmd/xo
TestStdChecks, the guard that every runtime body checks, sets it). `--support` keeps the module check cache and the lazy
runtime too, and builds its SSA through the front end cache (the driver
passes core the build as a closure, compiler/xo main.xo): 0.384x and
0.43x wall. Measured: `xo check` of a small server 0.432x wall
and 0.656x peak memory, of compiler/xo 0.369x and 0.669x; the same
output in 76 of 76 directories, every errorcheck program among them
(bench/results/check-cache-2026-10-23.md). Test: cmd/xo
TestCheckCacheWarnings.

## Addendum (2026-10-23): a cold build stores into the shared pack

Lead-approved (item A of the Windows --race note). A build with
COLD_PACK_MIN (64) or more new entries of a kind writes them as one
segment of that kind's shared pack (compiler/xo bcache.xo store_entries)
instead of a file each; a cold build of a small program left 638 files in
the cache instead of 6,227 and took 0.687 of the wall time on the Mac
(bench/results/cold-pack-2026-10-23.md). Parts stay in the target's pack.
A compaction of the shared pack keeps what the compacting build used, so
an entry another program needs may go and be built again (never wrong
code). XO_COLD_PACK=off stores loose as before.

Fix (2026-10-23, after the lead's Windows report): store_entries only
appended, and the compaction lived in shared_store, which runs when a
build read 64 or more entries loose; a packed cold build reads none
loose, so a cache that saw many xo versions (every CI run's) grew a
segment per version, and every build reads every segment. Eight
generations of a windows --race cross build (new keys each, same
program) left 114 MB in 11 segments, against 13.7 MB with
XO_COLD_PACK=off. store_entries now applies shared_store's rule (past
PACK_SEGS segments, or a pack holding more than twice what the build
used, one segment of what it used and made replaces the old): 26 to
51 MB over the same eight (bench/results/cold-pack-2026-10-23.md;
TestSharedPackBounded).

Opt in (2026-10-23 evening, lead): the instrumented windows-race.sh runs
(CI 40019736764 and 40019755932) put main's four builds at 564 to 623 s
each against 9.6 to 16.1 s for the 0.27.0 archive's xo, on the runner's
31 GB backend cache (16 GB cap), main removing 5.5 GB of it; the test
binaries ran alike (265.0 s against 265.2 s). On the Mac, two APFS clones
of a 17 GB cache with the prune inline (XO_PRUNE_DETACH=off, as on
Windows), at the cap and with an 8 GB cap, put both xos' builds at 4.6
to 19 s with bounded prunes. The cause is not found, so XO_COLD_PACK=on
opts in and a cold build stores loose as 0.27.0 does.

## Consequences

- Stage 1 changes the cache layout (a new `packs/shared` tree) but not
  any build's output (TestSharedPack: equal to a cache-off build).
- On by default since the owner's decision of 2026-10-23 (a default
  change: its gate ran the touched packages' full suites).
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
