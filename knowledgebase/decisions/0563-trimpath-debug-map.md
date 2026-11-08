# 0563: --trimpath and the darwin debug map's cache paths

Status: Proposed (Xo Lang 2, 2026-11-08; the owner decides, since either
option narrows a decided promise: decision 0354's or decision 0401's)

## Context

Decision 0354's addendum (2026-10-25) promises that no path of the build is
in a binary: --trimpath is on by default and records the DWARF directories
relative to the working directory. Decision 0401 gives a separately compiled
darwin link (decision 0388) a debug map instead of std's DWARF. Its N_OSO
entries name the program's objects, copied to the back-end cache's
`dbgobj/` by their SHA-256 (compiler/xo ld.xo `prog_debug_objects`), and
std's part objects beside its store (`std_parts_dir`). All are absolute
paths under the user's cache, so under $HOME. lldb reads them from there.
--trimpath does not touch the map.

A darwin build is separately compiled by default once std's variants are
cached. Since 02b82bb30 those variants are keyed by std's file contents,
so a fresh checkout hits the cache that earlier builds filled. Since then
a default darwin build carries $HOME in its debug map. The 0.44.0 release
build found it (2026-11-08): scripts/release.sh's path check refused the
darwin-arm64 xo, whose map named
`~/Library/Caches/xo/backend/dbgobj/<sha>.o`. 0.41.0's build had missed
the variants and built whole. cmd/xo TestBuildCarriesNoBuildPaths did not
catch it: the test tiers set XO_SEPCOMP=0.

Already done, outside this decision:

- scripts/release.sh exports XO_SEPCOMP=0, so a release builds whole
  whatever the cache holds. 0.44.0 was rebuilt that way.
- TestBuildCarriesNoBuildPaths gains a separately compiled darwin/arm64
  case (XO_SEPCOMP=1 with a private std cache). It requires today's leak,
  so main stays green until this decision lands.
- gotchas/debug-map-names-cache-paths.md.

## Options

(a) **--trimpath means no debug map.** A trimmed darwin link embeds std's
and the program's DWARF as a whole build does. Since --trimpath is the
default, 0401's map then exists only under --trimpath=off. This keeps
0354's promise for every build. Its cost is 0401's link time on every
default darwin build: Results below, measured with `xo bench --ab` once
the release window closed.

(b) **The map stays for local builds; shipping builds drop it.** A link
writes no debug map under --release or an explicit --trimpath (a
--trimpath the command line gives, not the default). release.sh passes
--trimpath and keeps XO_SEPCOMP=0. 0354 gains an addendum: a default
build's debug map names objects in the builder's own cache, as a dSYM
beside the binary would, which no other machine can use. The test's case
then requires no path under --release and under an explicit --trimpath,
and the cache path in a default build.

(c) Name the objects relative to a root lldb resolves. lldb opens an
N_OSO path as given, relative to its own working directory, so this
would need a source map or debug-file-search-paths setting per user.
Rejected: a debugger should work with no setup (decision 0252).

## Recommendation

(b). The map only points into the builder's own cache, so it is useless
anywhere else. The builds 0354 protects are the ones that leave the
machine (releases, --release, an explicit --trimpath). Default dev builds
keep 0401's link time.

## Decision

Pending the owner.

## Consequences

- (a): every default darwin build links std's DWARF again: the Results'
  link time, and the binary's size.
- (b): a default darwin dev binary still names `~/Library/Caches/xo`, and
  0354's text says so. A binary shared from a default build carries the
  builder's home path.

## Results

To measure after the release window (lead, 2026-11-08): `xo bench --ab` of
(a) against main on darwin/arm64, for xo's self build and one large
program.
