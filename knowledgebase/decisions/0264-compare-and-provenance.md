# 0264: xo dev compare and build provenance stamps

Status: Accepted (owner, 2026-10-18, through the lead: "xo dev compare
(two builds, callgrind, per-function join, side-by-side asm, JSON) and
build provenance stamps (bench scripts refuse mismatches;
XO_INLINE_STRICT=1 default in tests and bench)"); implemented 2026-10-18
by Xo Lang 3: stage 1, `xo dev compare` over ANN=1 runs (e8128205), and
stage 2, the stamps and the bench guard; the two builds and the
side-by-side assembly of item 4 are not done yet

## Context

Performance work on HTTP, TLS, and HTTP/2 (2026-10-16 to 2026-10-18)
compared two compilers by hand every time: build xo at two commits, run
bench/perf/http/callgrind-docker.sh on each, join the callgrind outputs
per function with scratch scripts (cgall.py, cgtop.py, cgcalls.py,
cginstr.py, ri.py), then dump and diff the assembly of the functions that
moved. Each step had its own failure:

- A measurement ran a stale xo: the binary under OUT/xo was from another
  commit, or a worktree's bin/xo had not been rebuilt after a rebase
  (feedback: fresh xo for new std). Only callgrind-docker.sh writes a
  commit stamp (OUT/xo/commit.txt), and nothing checks it.
- An xo whose arm64 inliner failed to verify built every program
  without inlining (slower, still correct) and the numbers looked like a
  regression of the change under test; XO_INLINE_STRICT=1 makes that an
  error, but only the Go tests set it.
- `xo version` says `xo 0.20.0 darwin/arm64`: not which commit, whether
  the tree was dirty, or which std it reads.

## Decision

1. Provenance stamp. xo records, at build time, the commit of the tree
   it was built from, whether the tree was dirty (a hash of the diff when
   it was), the source key xobuild computed, and the seed or xo that
   built it. `xo version --json` prints them with the version and host;
   `xo version` adds the short commit (`xo 0.20.0 (e9f04a2c, dirty)
   darwin/arm64`). The bootstrap and xobuild pass them in (an env var
   read by the build of compiler/xo, compiled in as constants).
2. Benchmark scripts refuse a mismatch. bench/perf scripts (callgrind,
   callgrind-docker, the conc and llvm runners) read `xo version --json`
   and refuse to run when the stamp's commit is not the checkout's HEAD,
   or the tree is dirty with a diff hash other than the checkout's
   (`XO_BENCH_ANY=1` allows it, said in the results). Every results file
   records the stamp.
3. XO_INLINE_STRICT=1 by default in the Go tests (already, internal/
   xotool) and in every bench script (set unless the caller set it), so
   an inliner fallback fails the measurement instead of skewing it.
4. `xo dev compare A B [--scenario=plain,tls,...] [--json]` where A and B
   are xo binaries (or commits, built through xobuild's cache): runs the
   HTTP callgrind matrix (MODE=short by default) with each, then joins
   the per-function inclusive and self counts (callgrind_annotate's
   output, names demangled the same way) and prints the functions that
   moved most, per scenario, with instructions per request for each side
   and the difference. `--asm=FUNC` prints the two builds' assembly of
   FUNC side by side (the arm64 or amd64 emitter's `--asm` output, cut to
   the function). JSON output has one object per scenario with the
   totals and the function rows, so an agent reads it without parsing
   tables.
5. Inputs and the helper it replaces (lead, 2026-10-18): `xo dev
   compare` reads the run directories of bench/perf/http with `ANN=1`
   (per-function callgrind_annotate counts per run, aeb48d3b) and
   replaces bench/perf/http/cgcompare.py (BASE/run NEW/run: per-request
   totals and the top changed functions, generalizing Xo lang 5's
   cgdiff); the owner wants profiling helpers in Xo. cgcompare.py's
   output is the cross-check while it lands, then it is deleted.

## Implementation notes (2026-10-18)

- Stamps: compiler/xo/provenance.xo declares XO_COMMIT, XO_DIRTY (the
  first 12 hex digits of the SHA-256 of `git diff HEAD`, "" when
  clean), XO_SOURCE_KEY, XO_BUILT_BY, set with `xo build --stamp`
  (an existing flag, so the 0.20.0 seed stamps the new compiler) by
  internal/xotool BuildXo (StampArgs, only when the tree has
  provenance.xo, so a seed chain hop of an older tree is not stamped)
  and by scripts/bootstrap-xo.sh (the commit and diff hash, the same for
  every stage so s2 and s3 stay identical; no source key, which needs
  Go).
- `xo version` alone is unchanged, not `xo 0.20.0 (e9f04a2c, dirty)
  darwin/arm64` as item 1 proposed: scripts/seed-pack.sh and seed-cut.sh
  read its words by position. `xo version --json` and `xo version -v`
  print the stamps.
- The bench guard compares source keys, not commits: a build is cached
  by source key and shared across commits and worktrees, so a cached xo
  keeps the commit that first built it while its sources are this
  checkout's; the key is what decides. bench/perf/xocheck.sh (sourced by
  bench/perf/run.sh, llvm/native.sh, conc/run.sh, and
  http/callgrind-docker.sh) refuses an xo whose stamp key is not `go run
  ./scripts/xobuild --key`, or that has no stamp, unless XO_BENCH_ANY=1;
  it sets XO_INLINE_STRICT=1 unless set and XO_STAMP, which the scripts
  print (callgrind-docker.sh into OUT/stamp.txt).

- `--callers=REGEXP RUN [SCENARIO...]` (2026-10-18, the lead's request
  after the HTTP allocation hunt): the callers of the functions REGEXP
  matches, per request, from the raw callgrind outputs a run keeps with
  KEEP=1; a caller that itself matches is left out, so the allocator's
  own layers do not hide who allocates. Symbols are demangled (arm64
  gen.xo fname), so a pattern and the output use Xo names.

## Consequences

- One command replaces the scratch tool chain and its mistakes; the
  stamps make a stale binary a refusal, not a wrong number.
- The stamp makes xo's bytes depend on the commit: the fixed point
  (s2 and s3 identical) still holds because s2 and s3 are built from the
  same tree with the same stamp; the stamp is left out of the source key
  so caching is unchanged.
- A dirty-tree build is visible as such everywhere it is measured.

## Alternatives

- A results-file convention only (write the commit by hand): what failed
  before.
- perf/Instruments on the Mac instead of callgrind: noisier, and not the
  numbers decision 0180 gates on.
