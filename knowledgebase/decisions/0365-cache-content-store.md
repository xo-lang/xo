# 0365: A content store behind the back-end cache's per-id keys

Status: Proposed (Xo Lang 3, 2026-10-24; for the owner's Sunday review,
nothing built). Deferred by the owner on 2026-10-24 ("measure a day of
churn after today's cache fixes first"); that day is measured below
(2026-10-26).

Compression: Accepted and built (owner, 2026-10-26, ~14:30: "port LZ4 block
into xo, internal codec, and compress the backend cache"; see "Built: LZ4
compression" below). The content store itself stays Proposed, to be
re-measured on the compressed cache.

## Context

The back-end cache keys every entry with the code generation key (cache_id,
bcache.xo: a hash of the backend's sources), so xos built from different
backend sources never share an entry. That is the safety property: a
different compiler is never served another compiler's bytes. Its cost, seen
in the store-log (ADR 0351 addendum): the cache stores 2,454 GB in 54.6 hours
at a 16 GiB cap, so it holds about 8.5 minutes of stores; the newest 16 GiB
came from 5 live ids storing at once (6.5, 5.5, 2.6, 1.7, 0.9 GB). Idle-id
eviction would free nothing (no bytes in that window from an id idle for 30
minutes; prune is already oldest-used first), so the lever is the volume each
live id stores.

**Measurement** (2026-10-24; scripts in the session scratchpad, results in
~/xo-lead/notes/lib1/resume.md): XO_CACHE_STATS=keys logs a sha256 of every
loose write and of every entry inside each pack segment (keys mode only;
cachestats.xo). Clean committed xos X1 and X2, one private cache, whole-build
reuse off, X2 differing from X1 only in compiler/arm64, so that it has its own
id and misses every entry. The bytes X2 stores that are byte-identical to
something X1 stored, for a cold build of compiler/xo (1,120 MB for X1):

| X2 differs by | X2 stores | identical to X1's | fn | inl | rc | obj | part |
|---|---|---|---|---|---|---|---|
| a comment | 1,833 MB | 899 MB (49%) | 100% | 100% | 100% | 100% | not yet stored by X1 |
| one peephole off (borrow_flag_peep) | 1,833 MB | 897 MB (49%) | 100% | 100% | 100% | 99% | not yet stored by X1 |
| copy_runs off (every function changes) | 1,887 MB | 489 MB (26%) | 31% | 100% | 100% | 2% | 0% |

(X2 stores 700 MB more than X1 in its cold build because X1's part packs form
on its second build; on the second build, with equal code, the part pack is
identical too: 713 MB of 713 MB.) http_server: 42%, 40% and 32% of a cold
build. The front end (fe) and the inliner and reference counting entries
(inl, rc) come before the backend, so a backend change shares them whole;
fn, obj and part share what the change does not touch.

So the second id re-stores the first's bytes at 26% (a change to every
function) up to 100% (a change to a few), while no entry could be shared by
key.

**Read cost** (uncached disk, files written with F_NOCACHE so the page cache
holds none, 8 KB bodies, 20,000 entries): a direct hit reads 115 microseconds
on one thread (26 at 8 threads); a hit through a pointer file (read the
pointer, then the body) 215 (45 at 8 threads). sha256 hashes 3.0 GB/s.

## Measured: a day of churn after the fixes (Xo Lang 2, 2026-10-26)

From the cache's store-log and prune-log (scripts/cache-churn.sh, read
only; the hourly rows are UTC, the day below is 2026-10-24 00:00 to 24:00
EDT; distinct ids, kinds and id lifetimes counted from the store-log
directly). Cap 16 GiB throughout.

| | 2026-10-24 (before the fixes) | last 24 h (to 2026-10-26 12:30 EDT) |
|---|---|---|
| stored | 1,406 GB | 1,097 GB |
| pruned | 674 GB | 381 GB |
| distinct ids that stored | 224 | 197 |
| stored by ids retired at the report (no store in its last hour) | 97 to 100% an hour | 86% |
| stored by ids that lived under 3 hours (first to last store) | 55% | 75% |
| stored by the 5 biggest ids | 27% | 20% |
| id lifetime, median and p90 | 0.0 h, 1.6 h | 0.1 h, 2.3 h |
| by kind (GB) | pack/part 419, pack/fn 273, pack/inl 192, pack/fe 167, pack/rc 109, obj 104 | pack/part 335, pack/fn 284, pack/fe 179, pack/inl 177, pack/rc 93, obj 88 |

Stored bytes count pack segments each time one is written, rewrites
included, which is why stored exceeds pruned by more than the cap; the
identical-bytes shares above were measured on the entries inside the
segments, so they apply to the same counting. The day after the fixes
stored 22% less, about what the lower agent activity explains (197 ids
against 224); per id the volume is unchanged (5.6 and 6.3 GB). The shape
did not change: three quarters of the bytes come from ids that live under
three hours (an agent's branch, a bisect step, a measurement's two xos),
each re-storing what the ids before it stored. At today's rate the cap
holds about 22 minutes of stores (16 GiB, 17.2 GB, at 45.7 GB an hour).

What the measured shares would save on today's churn: 26% (every function
changed) to 49% (a small backend change) of the 1,097 GB, so 285 to 537 GB
a day fewer bytes written, and the cap would hold about 30 to 44 minutes of
stores instead of 22. The part and fn packs, 53% of the stored bytes (the store-log's count), are
where the shares were highest (part packs 100% identical on equal code).

### The alternative measured: compressing the cache (Xo Lang 2, 2026-10-26)

A 2.2 GB sample of the real cache (copied read only, deleted after: the
newest and oldest segments of each kind; part 829 MB in 7 segments, fn
498 MB, fe 242 MB, inl 231 MB, rc 186 MB, obj 401 MB in 501 files), each
file compressed alone, one core (`zstd -b -T1 -i2`, `lz4 -b`), in a
measurement window at load 2 to 3:

| area | zstd -1 ratio, MB/s c / d | zstd -3 | zstd -6 | lz4 -1 |
|---|---|---|---|---|
| part | 13.5x, 1,480 / 3,138 | 13.2x, 1,289 / 3,024 | 16.8x, 354 / 3,625 | 6.4x, 1,762 / 7,038 |
| fn | 9.6x, 1,142 / 2,662 | 9.8x, 994 / 2,772 | 11.7x, 304 / 2,818 | 4.6x, 1,236 / 6,727 |
| inl | 3.4x, 845 / 1,713 | 3.8x, 560 / 1,511 | 4.2x, 187 / 1,540 | 2.3x, 1,144 / 6,933 |
| rc | 3.4x, 853 / 1,707 | 3.9x, 573 / 1,531 | 4.2x, 189 / 1,567 | 2.3x, 1,176 / 7,082 |
| fe | 3.2x, 796 / 1,667 | 3.6x, 509 / 1,488 | 3.9x, 174 / 1,562 | 2.2x, 1,097 / 6,576 |
| obj | 3.9x, 785 / 1,626 | 4.2x, 621 / 1,523 | 4.4x, 200 / 1,529 | 2.5x, 1,069 / 6,094 |

On the last 24 hours' stores by kind (the store-log's part 335, fn 284,
fe 179, inl 177, rc 93, obj 88 GB, the rest 21 GB taken at 3.5x), scaled
to the 1,097 GB of the churn table:

| | written a day | saved a day | compress CPU a day | warm llvm-fib build's reads |
|---|---|---|---|---|
| zstd -1 | 204 GB | 893 GB (81%) | 1,080 s (18 core minutes) | 13.0 ms |
| zstd -3 | 187 GB | 910 GB (83%) | 1,440 s | 13.9 ms |
| zstd -6 | 168 GB | 929 GB (85%) | 4,490 s (75 core minutes) | 13.2 ms |
| lz4 -1 | 330 GB | 767 GB (70%) | 866 s | 4.2 ms |

The hit path: a warm build of bench/perf/llvm/xo/fib (whole-build reuse
off, XO_CACHE_STATS=1, xo at origin/main) reads 28.3 MB from the cache
(4.6 MB of loose entries, 13.4 MB of its pack, 10.3 MB of the shared
pack); at the mixes' decompress speed that is the time in the last column,
against a warm build of about 300 ms, and the reads themselves shrink by
the ratio. Part and fn segments compress so well (13x and 10x) because a
segment repeats the same functions' code and names; this is the bytes the
content store would share across ids (49% at best), which compression
removes within one id's segments as well. The cap would hold about five
times the history (16 GiB of compressed segments).

### Implementation note: compression in xo (Xo Lang 2, 2026-10-26; nothing built)

**The codec.** xo writes the cache itself, so the codec must be Xo code
(xo links no C library; zero C runtime). std has only DEFLATE today
(std/compress: gzip, flate, zlib, a port of Go's compress/flate in
std/native/llrt/flate.xo, 2,079 lines); no zstd, lz4 or snappy. Measured
on a 450 MB sample of the same cache (newest part, fn, fe packs and obj;
Xo program built --release, one core, measurement window at load 4):

| area | Xo deflate -1: ratio, compress / inflate MB/s | deflate -6 | zstd -1 CLI, same files |
|---|---|---|---|
| part | 8.3x, 194 / 266 | 12.2x, 68 / 287 | 11.8x, 1,313 / 2,830 |
| fn | 4.5x, 126 / 218 | 6.2x, 45 / 246 | 6.3x, 814 / 2,052 |
| fe | 2.8x, 100 / 181 | 3.4x, 49 / 193 | 3.3x, 810 / 1,688 |
| obj | 2.9x, 106 / 201 | 3.2x, 40 / 214 | 3.5x, 746 / 1,511 |

(C gzip -1 compresses these at 160 to 450 MB/s, so the Xo port runs at
about half of C.) DEFLATE in Xo would save about 75% of the bytes, but at
about 230 MB/s inflate a warm llvm-fib build's 28.3 MB of reads would cost
about 120 ms on a 300 ms build, and compressing the day's 1,097 GB about
2.6 core hours: too slow for the hit path. A faster codec has to be ported:

- **LZ4 block format** (recommended first): Go's pierrec/lz4 block code is
  about 300 lines to compress (one fast level, a hash table of 4 byte
  matches) and about 100 to decompress (literal runs and back copies,
  mostly memmove), plus the frame we need (a length header; no LZ4 frame
  format, the cache is our own). With tests against Go's output, a day or
  two. Ratio 2.2 to 6.4x by the CLI (70% of the bytes saved); its decoder
  is copies, which Xo does at memmove speed, so it should land near 1 GB/s
  or better where DEFLATE's bit decoding ran at 230 MB/s: about 30 ms of a
  warm build at worst. Measure the port before switching it on.
- **zstd**: Go's standard library has a decoder only (internal/zstd, about
  2,000 lines with FSE and Huffman), and an encoder means klauspost's
  (the fastest level and its entropy coders, about 4,000 more lines): one
  to two weeks, for 81% against lz4's 70%. A second step if the 11% is
  wanted.

**Hooks.** Every cache file goes through compiler/xo bcache.xo: writes
through `cache_put(os, path, data)` (loose entries, `entry_path`, and
pack segments, `pack_write_counted`'s `s-` files), reads through
`cache_get(os, path)` (loose entries; obj entries are read into memory and
written out for the link, so a compressed .o costs no extra step) and the
segment reads in `cache_get_packed` and of the shared pack (`os.fs.read`
of each segment, then `pack_index`). Compress whole segments (one call per
segment, where the ratios above were measured) and each loose entry;
decompress right after the read, before `pack_index` or the entry's own
version check.

**Format and versions.** A compressed file starts with a magic and codec
byte (`XOZ` + 1 for lz4) and the uncompressed length; a reader that finds
no magic takes the bytes as they are, so entries written before stay
readable. Older xos must never read a compressed file as if it were plain
(a pack index parsed from compressed bytes could name wrong entries):
compressed segments get another name prefix (`z-`; cache_get_packed reads
only `s-` names today, so an older xo skips them), and compressed loose
entries another key salt (entry_path's key includes a codec tag), so an
older xo misses them instead of reading them. Entries of a code generation
key shared across xo builds (decision 0275) then split by codec for a
while; the old ones age out through the prune.

**Shared packs and prune.** The shared pack copies segment files byte for
byte, so a compressed segment stays compressed there and is read the same
way. Prune counts files by their size on disk, so the 16 GiB cap holds the
compressed bytes (about five times the history); its walk, KEEP and
version guard are unchanged. XO_CACHE_COMPRESS=off would write plain
files (reads always accept both), for a measurement or a bisect.

### Built: LZ4 compression (Xo Lang 2, 2026-10-26)

**Codec.** LZ4 block format, std/native/llrt/lz4.xo (the reference fast
compressor's greedy matcher: a hash table of 4 byte sequences sized to
the input, 2^10 to 2^16 slots, the search step growing every 64 misses,
backward extension; the decoder checks every length and offset and uses
8 byte copies into 32 bytes of slack, as lz4.c's wildcopy). It is
`internal.lz4_compress` and `internal.lz4_decompress(src, start, n)` in
std/internal (toolchain only, not std's public surface); std/rt's
internal_lz4.go is the same algorithm for the Go backend. Tests:
internal/lz4test builds testdata/tool on arm64, LLVM and the Go backend
and checks that each writes the Go copy's bytes for a corpus (the
format's edge lengths 0 to 20, runs, random, long literal and match
runs, offsets 65535 and 65536, text, the repository's sources), that Go
reads them back, and that on 6 cut and 6 flipped copies of every block
both decoders agree (the same bytes or both None); with the lz4 tool
installed, `lz4 -d` reads every native block and the native decoder
reads `lz4 -1` and `-9` blocks.

**Speed** (one core, measurement windows; `lz4tool bench`, 30 rounds; the C
lz4 tool's `-b1` on the same files):

| file | ratio (C) | compress MB/s (C) | decompress MB/s (C) |
|---|---|---|---|
| part segment, 13.4 MB | 5.75 (6.17) | 1,437 (1,732) | 2,947 (6,472) |
| fn segment, 1.2 MB | 5.85 (6.20) | 1,619 (2,320) | 4,549 (11,921) |
| fe segment, 3.0 MB | 1.96 (1.86) | 344 (985) | 2,103 (7,261) |
| obj entry, 223 KB | 2.13 (2.12) | 639 (1,529) | 2,849 (6,539) |

The decoder's short copies took it from 0.8 to 2.2 GB/s to the speeds
above.

**Format.** `XOZ`, a codec byte (1 lz4; 0 stored, when lz4 would not
shrink the body or it is under 32 bytes), the body's length (varint),
the body. A loose entry is `z<KEY>` beside `<KEY>` in its shard; a pack
segment is `z-<ID>` beside `s-<ID>` (compiler/xo zcache.xo: entry_get,
entry_put, is_segment, segment_bytes). An older xo looks only for
`<KEY>` and `s-` names, so it misses compressed files instead of
misreading them. With compression on (the default) a build reads `z`
entries only (one open each) and segments of both kinds;
`XO_CACHE_COMPRESS=off` reads and writes plain files. Plain entries of
the same code generation key age out through the prune. Not compressed:
the module check cache (chk), the fe-bad copies, and the small state
files (seen, neg, hints, memos, logs).

**Hit cost** (bench/perf/llvm/xo/fib, private caches per mode warmed by
four builds, 15 interleaved rounds, medians; measurement window at load
3): a warm build with whole-build reuse off 250 ms compressed against
241 ms plain (+9 ms; the bar was 20 ms); an edit rebuild (one new
function each round) 278 against 270 ms (+8 ms). The cache after the
runs: 12.1 MB compressed, 34.1 MB plain (65% less).

**Still to do:** re-measure the day's churn (the store-log) on
2026-10-27 with compression on, then the content store's share on the
compressed cache.

### Recommendation (Xo Lang 2, for the owner)

Build it, starting with the part and fn packs. The fixes of 2026-10-24
lowered the volume only as far as activity fell, and the churn is still
three quarters short-lived ids re-storing bytes another id already wrote;
that is the case this decision targets and nothing else in the cache
addresses (idle-id eviction frees nothing, as the Context says). The gain
is about 0.3 to 0.5 TB a day less written to the Mac's SSD and a cache
that remembers up to twice as long, against one more read per pointer
hit and a reference sweep in prune; stage it so the sweep is tested with
fault injection before the fn, inl and rc packs move over, and measure
the hit rate of an agent switching between two branches before and after.
If the owner prefers less risk, keep it deferred and add zstd compression
of the segments first (additive, no sweep); closing it would leave the
churn as measured here.

Revised after the compression measurement above (the same day): compress
first. zstd at level 1 or 3 removes 81 to 83% of the bytes written (about
900 GB a day) for under half an hour of one core a day and about 14 ms of
a warm build, with no change to keys, pruning or sharing; the content
store's 26 to 49% applies to what is left, and is worth re-measuring on
the compressed cache before it is built.

## Decision

Proposed, not built. Keep every key per id; store the bytes once by content.

- An entry's file in the id's key space becomes a pointer (the content hash,
  32 bytes) for bodies over a threshold (small entries stay inline); the body
  lives once in `objects/<xx>/<hash>`.
- A store hashes the body; when `objects/<xx>/<hash>` exists it writes only the
  pointer (no body write, which is the saving), else the body (temporary name
  plus rename, as cache_put) then the pointer.
- A hit reads the pointer, then the body, and checks the body's hash only in
  tests (the body is addressed by its hash, written whole by rename).
- Pack segments hold the pointers for their entries (or inline small bodies);
  the part packs, whose bodies are the 713 MB of compiler/xo, point into the
  object store.
- Prune: pointers are entries as today (oldest used first, KEEP, the cap). A
  body is removed only when nothing points at it, so a prune also sweeps the
  bodies: it walks the pointers it keeps (it already walks them), builds the
  set of live hashes in a bounded pass (sliced like the shard walks), and
  removes the bodies not in the set and older than KEEP. A body is always
  written before its pointer, and a body younger than KEEP is never removed, so
  a store racing a sweep keeps its body.

## Consequences

- Stored bytes fall by the shared share of the 2,454 GB: between 26% and 100%
  of what a second id stores, so the 5 live ids of the store-log hold much
  more history in the same cap.
- Each hit through a pointer costs one more open and read: +100 microseconds on
  one thread uncached (+19 at 8 threads), a hit in the page cache a few
  microseconds. The compiler reads about 6,000 entries for hello, but through
  the shared pack (no per-entry opens); the pointer cost applies to loose
  entries (obj, a few thousand) and to pack hits only when a segment points
  out. Measure before building.
- A store hashes its body: 1.1 GB for a cold compiler build is 0.4 s of one
  core at 3 GB/s, against the 25 s it takes.
- Prune gets a reference sweep, the one real complexity; a bug there loses
  entries (a miss, never a wrong hit: bodies are addressed by their hash).
- Safety is unchanged: a key belongs to one id, and the pointer's body is
  addressed by the hash of its bytes.

## Alternatives

- Key entries by their inputs only (no id): shares everything and is wrong
  whenever the backend's output depends on code the key leaves out. Rejected
  (the property the id exists for).
- Hard links for equal bodies: needs the hash anyway and breaks on a volume
  without links (Windows CI).
- Compress segments (zstd): shrinks every id's bytes by a constant, shares
  nothing; additive to this, measured separately.
- Do nothing: the cap stays 8.5 minutes of stores for 5 ids.
