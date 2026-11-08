# A separately compiled darwin build names the cache in its debug map

Status: Observed (2026-11-08, the 0.44.0 release build; Xo Lang 2)

## Symptom

The first try of the 0.44.0 release build (scripts/release.sh) stopped
with this error:

    release.sh: .../dist/xo-v0.44.0-darwin-arm64/bin/xo holds the path /Users/<user> (--trimpath)

The darwin-arm64 xo contains strings such as
`prog-darwin.0.o /Users/<user>/Library/Caches/xo/backend/dbgobj/<sha>.o`,
even though --trimpath is on. 0.41.0's build with the same script passed.

## Cause

Separate compilation (decision 0388) is the default once std's variants
are cached. Since 02b82bb30 those variants are keyed by std's file
contents, so a fresh release checkout hits the cache that earlier
builds filled. A separately compiled darwin link writes a debug map
(decision 0401): N_OSO entries name the program's objects, copied to
the back-end cache's dbgobj/, and std's part objects, both by absolute
path. --trimpath (decision 0354) rewrites the DWARF directories, not the
debug map. 0.41.0's build missed the variants and built whole, with no
debug map.

cmd/xo TestBuildCarriesNoBuildPaths did not catch it: the test tiers set
XO_SEPCOMP=0.

## Fix

- The 0.44.0 build was rerun with XO_SEPCOMP=0 (rel-final.sh). The race
  runtimes from the first try were reused.
- scripts/release.sh exports XO_SEPCOMP=0, so a release never depends on
  the cache state.
- To check any shipped binary: `grep -aqF "$HOME" BIN`. The `strings -a`
  output can miss the entries.

## The same build's second leak: the prebuilt SQLite objects

The rebuild with XO_SEPCOMP=0 passed release.sh. A check of every file in
every archive then found `lib/xo/sqlite/darwin-{arm64,amd64}/sqlite.o`
(new in 0.44.0, decision 0534 stage 2) naming the release checkout in
their DWARF. scripts/sqlite-prebuilt.sh compiled them with `cc -g -O2`
and no prefix map, and release.sh checked only the bin/xo files.

0.44.0 was rebuilt a third time with a cc wrapper that adds
`-ffile-prefix-map=<checkout>=.` to compiles. On main:

- sqlite-prebuilt.sh passes `-ffile-prefix-map=$repo=.` for darwin and
  linux.
- release.sh refuses any staged file, of any target, that holds the
  checkout's or the home directory's path, before it packs the archives.
