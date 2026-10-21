# 0297: Binary and test-run cache keys by build inputs, not xo's identity

Status: Proposed (2026-10-20, Xo Lang 3; a draft for the lead and the
owner; the measurement below runs first, on CI with the owner's
approval)

## Context

Decision 0275 keyed the backend caches (front end, check, rc, inline,
fn, part, object) on XO_CODEGEN_KEY, a hash of the code generation
sources stamped into xo, so every xo built from the same backend
sources shares their entries. It left two caches on self_id, the
SHA-256 of xo's own executable (compiler/xo bcache.xo):

- the binary cache: macho_bin_key (a link by compiler/macho) holds
  self_id because xo is the linker; native_bin_key (a link by cc) and
  the Go backend's key hold the tools' identities and the link inputs;
- the test-run cache (testcache.xo run_key, decision 0236 addendum):
  a test binary and its test list, found before the check, keyed by
  self_id, the command line without run-only flags, the XO_
  environment, the C toolchain, the SDK and the runtime pack. The entry
  also lists every file the build read (the std tree, each module
  directory, the directory listings, xo.mod and the lock and work
  files) with its SHA-256, and is used only when all still match.

Every xo has a new self_id: any source change, a different seed, a
provenance stamp (XO_COMMIT, decision 0264). So each gate's new xo, and
each developer build after a pull, starts both caches cold. 0275's
corrected Consequences place the Mac's 767 s cold against 349 s warm
for internal/arm64 (2026-10-20) on exactly these two caches: the 349 s
came from a second run with the same xo. 0275 itself left a suite's
wall unchanged (CI, internal/arm64: 889 s against 926 s).

## Decision (proposed)

Key both caches on a stamped build key, XO_BUILD_KEY, in place of
self_id, failing closed to self_id as 0275 does:

- **XO_BUILD_KEY** is the SHA-256 of the code generation key's inputs
  (0275's import closure and VERSION) plus the driver: the packages
  whose code runs when xo builds, links or runs a test. Those are
  compiler/xo, compiler/gen (the Go backend), compiler/prune,
  compiler/cli/core (0275 keys only its format), and
  compiler/cli/modules (module resolution). xobuild and
  scripts/bootstrap-xo.sh stamp it as they stamp XO_CODEGEN_KEY
  (internal/xotool CodegenKey gains BuildKey).
- **Left out:** packages that only serve other commands:
  compiler/cli/semantic (def, refs, type, doc, symbols, query),
  compiler/cli/servers (the language server), compiler/cli/dev, and
  compiler/debug (dbg). An edit there rebuilds xo but keeps every
  binary and test-run entry. The provenance stamps (XO_COMMIT and the
  rest) are left out too: they change no output.
- **macho_bin_key** and **run_key** use "build ${XO_BUILD_KEY}" when it
  is stamped (64 hex digits), else self_id. native_bin_key and the Go
  key are unchanged (no xo identity in them).

## Safety net

0275's two conditions carry over:

- **Classified inputs.** internal/xotool's codegen test lists every
  environment variable, Module field and data file the code generation
  reads as keyed or not. It extends to the driver's reads: every
  os.env.get, os.fs read and command line flag in the included
  packages is in the key (the run key's XO_ environment and command
  line already are) or listed as run-only with a reason.
- **The boundary is checked.** compiler/xo imports the left out
  packages only to dispatch their commands (the files that `use`
  compiler/cli/semantic, servers or dev). A test fails when any other
  file of an included package imports a left out one, so the build,
  link and test paths cannot reach them. A left out package that starts
  to affect output then fails the test instead of serving stale
  entries.
- **Fails closed:** an unstamped xo (an xo built by hand, a seed) keys
  on self_id, as today.

## Measurement first

Owner approval per CI use. On one linux/arm64 runner:

1. internal/arm64 full tests twice with one xo: the cold and warm
   walls, as the Mac's 767 s and 349 s.
2. A tool-only change (a comment in compiler/cli/semantic), xo rebuilt,
   the suite again: today this is cold. The proposal is worth landing
   if this wall is near the warm one once the key is in.
3. The share of the 569 compiler/ or std/ commits of 2026-10-13 to
   2026-10-20 (0275's count) that touch no included package: how often
   a gate would start warm.

## Measurement result (2026-10-21, CI run 39676596988)

Runner xo-arm64-big2 (32 CPUs), xo 0.24.0, `scripts/ci/m297.sh`
(branch m297): TestGoldenRun alone (275 tests), `go test -json` per
pass, `du` and a file count of every cache directory before and after
each pass (cache root /home/ec2-user/.cache/xo). Raw files:
cache-stats.txt, times-*.txt, suite-*.json, m297.txt.

| pass | wall | failed | backend/ files added | testbin/ files added |
|---|---|---|---|---|
| cold (first pass) | 49 s | 0 of 275 | +72,670 (+1.5 GiB) | +275 (+0.67 GiB) |
| warm (same xo) | 41 s | 0 of 275 | +1,644 | 0 |
| after a tool-only change (a comment in compiler/cli/semantic, xo rebuilt in 95 s) | 42 s | 0 of 275 | +2 | +127 (+113 MiB) |

What it shows:

- The tool-only change today misses the binary and test-run caches for
  127 entries (testbin/ +127, keyed on xo's identity)
  but not the backend caches (+2 entries: 0275's codegen key held). The
  suite's wall stayed 42 s against 41 s warm. On this runner the part
  0297 would save costs about one second of wall, so the
  proposal's gain is not visible here.
- The first pass was not cold: the runner's cache already held 6.7 GiB
  and 428,614 backend files from other runs, so 49 s is a mostly warm
  start (72,670 new entries were still created). A truly empty cache
  was not measured; the Mac's 767 s cold against 349 s warm is the
  only cold figure.
- The xo rebuild pruned the backend cache from 502,928 files and
  21.1 GiB to 127,785 files and 16.5 GiB before the third pass, without
  costing hits (the third pass added 2 entries). The prune frees by
  file count far more than by size, which is worth its own look.
- The 2026-10-20 run (2442 s warm against 2414 s cold) was not usable:
  runner without the LLVM linker plugin, Windows identity diffs and a
  websocket flake. Its wall time is not comparable to this run's.

Reading for the owner: 0297 is not justified by CI's 32 CPU runner. The
case rests on the Mac (24 GB, few cores) where the same suite is
349 s warm and the tool-only change is cold today. The next experiment
is that Mac measurement: TestGoldenRun with XO_CLI set, once warm, then
after the comment change and an xo rebuild, with the cache counts
before and after (`scripts/ci/m297.sh`'s cache_stats on the Mac's
cache root, one bulk slot). If the Mac's third pass is near the warm
one only with the key in, build the key behind the safety net above.

### The Mac (2026-10-21, bulk run, empty private cache)

The same script on the development Mac (24 GB), `HOME` set to a private
directory so the first pass starts with no cache but the xo itself
(`TestGoldenRun`, 275 tests; passes in order, same xo unless noted):

| pass | wall | backend/ added | testbin/ added |
|---|---|---|---|
| cold | 123 s | +67,444 files, +1.3 GiB | +550 files, +665 MiB |
| warm (2nd) | 114 s | +1,645 files, +12.1 GiB | 0 |
| warm2 | 52 s | 0 (the prune freed 2.0 GiB) | 0 |
| warm3 | 43 s | 0 | 0 |
| tool-only change (xo rebuilt in 329 s) | 51 s | +81 files, +23 MiB | +548 files, +665 MiB |
| tool-only change, 2nd pass | 90 s | 0 | 0 |

(A first run of the same script gave cold 115 s, warm 107 s and a
tool-only change at 37 s; the walls swing by ten seconds or more with the
other agents' load on this machine.)

- The binary and test-run entries (testbin/) are all missed after the
  tool-only change (548 new entries), and that pass took 51 s against
  43 to 52 s for the two hot passes. The most the proposal could save
  here is the difference to the hot wall, about 8 s of 51 (below the
  spread between warm2 and warm3).
- The second pass over an xo is the slow one (114 s, no faster than
  cold): it writes 12.1 GiB of pack segments (decision 0220 addendum
  part 2d) for 1,645 files. The same cost explains the 90 s second pass
  after the tool-only change. That write is the large item in this suite
  and is not 0297's.
- Cold at 123 s on the Mac against 767 s of the earlier figure in this
  decision: the earlier figure was a different set (the package's whole
  test run).

Reading: for `TestGoldenRun` on either machine the binary and test-run
keys by xo's identity cost little (1 s on CI, about 8 s on the Mac)
because the backend caches (decision 0275's key) already carry the
tool-only change. 0297 stays Proposed; it is not worth its safety net on
this evidence. The next experiments: (1) the pack write above (packing
from a target's third build, or by hard link; Xo lang 5 has it); (2) the
same measurement on a suite where the test binaries' link dominates (the
package's whole run, `./internal/arm64` without `-run`), if the owner
wants the question closed for good.

## Consequences

- A gate whose change leaves the driver and the code generation alone
  (std, tests, docs, tools) runs its suites against warm binary and
  test-run caches. That is the 0275 gain the suites did not get.
- A change to an included package still starts both caches cold, as
  today.
- The same hole as 0275: a driver bug that changes output while the
  keyed sources do not would serve stale entries until verify mode or
  a golden catches it. The fixed point is the evidence that same
  sources build the same xo.

## Alternatives

- **Keep self_id** and accept a cold suite per gate: the 767 s
  against 349 s on the Mac.
- **Key on all of compiler/**: simpler, with no boundary test, but a
  tool edit (the most common change in the 0261 tools work) still
  colds the caches.
- **Key the test-run cache on the test binary's hash after the link:**
  exact, but a hit then needs the check, SSA and link it exists to
  skip.
