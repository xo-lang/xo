# 0418: Pack segments compress and stream per entry, and a build stores each part's entries as it finishes

Status: Accepted (xo-lang-6, 2026-10-30; the lead's go on the plan 2026-10-29 20:45)

## Context

The backend cache's packs (decision 0351, compressed by 0365) keep a target's entries in segment
files: one header (count, keys, lengths) and one body of every entry's bytes, LZ4 compressed as a
whole. Writing a segment builds the whole uncompressed body in a builder and then its compressed
copy; reading one decompresses the whole of it to serve any entry. The function and part entries of
a build (the function's text as Bytes plus its wants; the part's asm) are gathered in Program and
written after emit, so a cold build holds every function's text twice and every part's asm twice
from gen_part until fn_store. Measured 2026-10-29 (main's xo, the self build, same tree): footprint
cache off 8284 MB, cold cache on 9705, warm 6059; the warm build's fn_store alone adds 0.9 GB of peak.

## Decision

1. Segment format v2 (compiler/xo packseg.xo; PACK_VERSION "xo pack v2"): a segment file (p-<hex>)
   is the entries' bodies in order, each as it is or as one LZ4 block (whichever is smaller), then
   an index (the version line, the count, each entry's key, raw length, stored length and codec)
   and a 12 byte footer naming the index's offset. A writer streams the bodies to the file as it
   compresses them, holding at most 4 MiB; a reader keeps the file as read and decompresses only
   the entries it wants.
2. PACK_VERSION is part of every pack directory's name (pack_dir, shared_pack_dir), so the bump is
   a clean miss both ways: an older xo never lists a v2 segment and this xo never lists a v1 one;
   the old directories age out through the prune.
3. Each emit worker writes its part's new entries (the functions' records and the part's own) to
   one file in the build's private directory as gen_part returns (spill_part, parallel.xo), keeping
   only (key, offset, length) in Part.spill; the bytes leave the heap with the part. The driver's
   stores (fn_cache_store, pack_store, store_entries, shared_store) take a StoreSrc: the entries
   still in memory (XO_BACKEND_CACHE=verify, which compares them; a failed spill) and the spilled
   ones, read back one part at a time as the segment writer reaches them. The private directory's
   removal after the link removes the spills.
4. The hits a compaction rewrites (fc.entries) stay in memory as before: they were read for the
   build anyway.

## Consequences

- Measured on the self build (xo building compiler/xo of the same tree, 2026-10-30 00:00, the Mac
  loaded by other gates, so each xo is compared with its own cache-off run; A main's xo 0.34.0 at
  e0295d2e, B this change; /usr/bin/time -l footprint, --phase-times heap peak):

  | build | A footprint | A heap peak | B footprint | B heap peak |
  |---|---|---|---|---|
  | cache off | 7924 MB | 11208 MB | 8350 MB | 11148 MB |
  | cold, cache on (every entry stored) | 10428 MB (+2504 over off) | 13569 MB | 8371 MB (+21 over off) | 11510 MB |
  | second build (functions hit, parts stored) | 5809 MB | 7655 MB (fn_store) | 4807 MB | 5237 MB |
  | third build (parts hit) | 4300 MB | 4695 MB | 4488 MB | 5004 MB |

  The cold cache-on cost (2.5 GB of footprint, 2.4 GB of heap peak) is gone; the second build's
  fn_store peak falls by 2.4 GB of heap and 1.0 GB of footprint. Wall and CPU of the second build
  6.6 s / 21.4 s to 5.3 s / 9.7 s, of the third 5.4 s / 16.4 s to 4.4 s / 7.1 s (a read decodes
  only the entries it wants). The third build's +0.2 GB of footprint is within the run to run
  spread of the loaded machine.
- The cache directory after the three builds: 567 MB (v1) to 624 MB (v2): one LZ4 block per entry
  compresses about 10% less than one block over the segment.
- A format change: the old directories are not read; the first build after the bump stores every
  entry again (as any cache identity change does), and the prune ages the old directories out.
- XO_CACHE_STATS counts a segment once more (SegWriter.end writes the finished file through
  fs.write so StatFs sees it; keys hashes each entry from the v2 index, cachestats.xo note_items).
- Tests: compiler/xo packseg_test.xo (round trip, damaged files), internal/arm64's pack and cache
  tests (TestSharedPack*, TestFrontCacheNegKeys, TestBackendCache*, TestPrune*, TestCacheStoreLog,
  TestFrontCache*; the segment globs are [psz]-*), cmd/xo TestCacheStats, TestCodegenInputsClassified
  (arm64/parallel.xo classified: the spill is written, never an input).

## Alternatives

- Spill only, v1 format kept: moves the entries from emit to fn_store, where pack_write needs the
  whole body in memory; no peak gain.
- Smaller entries (no text in the function record): the record is the text; nothing to drop.
- A writer task streaming into a v1 segment: still one LZ4 block over the whole body.
- Mapping segments instead of reading them (0402's read_mapped): a later step; the compressed file
  is small next to what v1 decompressed.
