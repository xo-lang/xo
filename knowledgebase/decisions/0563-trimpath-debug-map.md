# 0563: --trimpath and the darwin debug map's cache paths

Status: Accepted (owner, 2026-11-08: option (b); drafted by Xo Lang 2.
Decision 0354 gains an addendum.)

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

(b), accepted by the owner on 2026-11-08. A darwin link writes no debug map
under --release or a --trimpath the command line gives (compiler/xo opts.xo
`ship_build`, `Ctx.no_debug_map`); it embeds std's and the program's DWARF
as a whole build does, and the bin cache key gains `debug-map=off`. A
default build keeps 0401's map, which names the builder's own cache.
Decision 0354's addendum of 2026-11-08 records the narrowed promise. cmd/xo
TestBuildCarriesNoBuildPaths' separately compiled case requires no build
path under --release and under an explicit --trimpath, and the cache path
in a default build.

## Consequences

- (a): every default darwin build links std's DWARF again: the Results'
  link time, and the binary's size.
- (b): a default darwin dev binary still names `~/Library/Caches/xo`, and
  0354's text says so. A binary shared from a default build carries the
  builder's home path.

## Results

Option (a) (branch l2-0563a: `Ctx.no_debug_map = trimpath_eff(o)`, the bin
cache key gains `debug-map=off`) against its parent c6b3a9d6b, darwin/arm64
(the Mac), a warm rebuild as ab-host.sh's edit mode does it (private HOME, a
back-end cache per side, std half on, no bin or run cache, no background
separate compilation), `xo bench --ab`, 2026-11-08:

| program | wall | cpu | max RSS | link phase (one build) | binary | $HOME in binary |
|---|---|---|---|---|---|---|
| internal/llvm golden http_server_request (n=11, load 9.8) | 0.997 (0.958 to 1.015, within noise) | 0.998 | 1.025 (1.005 to 1.032, worse) | 13.7 to 14.2 ms | 5,597,346 to 7,366,738 bytes (+32%) | yes, then no |
| compiler/xo, the self build (n=7, load 28.6) | 1.129 (0.919 to 1.471, within noise: spreads 12% and 21%) | 1.009 (0.958 to 1.038) | 0.998 | 835 to 702 ms | 87,035,154 to 85,345,538 bytes | yes, then no |

On these numbers (a) costs no build time that the measurements can see.
The cost is in the binary: a small program grows by a third, since std's
DWARF is embedded instead of mapped, and peak memory rises 2.5%. The self
build ran on a loaded Mac (load median 28.6), so its wall row settles
nothing; its cpu row is flat. To rerun in a quiet window:
`~/xo-lead/notes/xo-lang-2/ab-0563a.sh compiler/xo 11`. Logs:
~/xo-lead/agents/l2-ab0563b-cmd.log.
