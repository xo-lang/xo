# A cached build of xo can be separately compiled, so its code depends on the cache

Status: Observed (2026-11-12, Xo Lang 3)

## Symptom

The same xo building the same compiler/xo source gave a `__text` of
37,499,624 bytes with the shared back-end cache and 38,293,324 with
`XO_BACKEND_CACHE=off XO_FE_CACHE=off`. The closure numbers differed
(`RecEnv.note.fn187` against `fn185`), std calls such as
`prelude.str_starts_with` stayed calls where the other build inlined
them, and identical bodies were not merged into `_xo_h...` symbols. A
private, empty cache gave the same bytes cold, warm, and off, so the
difference looked like a cache that replays wrong code.

## Cause

Not the front-end or back-end entry caches: decision 0388's separate
compilation, on by default since 2026-10-28 (compiler/xo sepenv.xo
sepcomp_default). A native, non-`--release` build for the host links
std's prebuilt object (std/<key>.store in the back-end cache) once the
cache holds one for this xo, instead of building std with the program.
The first build finds none, builds the whole program, and starts std's
object in the background. Later builds link it. Turning off each other
cache (fe, bin, prep, ifc, edges, chk) changed nothing, while
`XO_SEPCOMP=0` gave the uncached build's bytes.

The code is correct, only built differently. The separately compiled xo
built compiler/xo to `__text` byte-identical with the whole program
xo's output, and every arm64 golden passed with it as `XO_CLI`.

It matters for seeds: scripts/release.sh exports `XO_SEPCOMP=0` (the
0.44.0 debug map failure, debug-map-names-cache-paths.md), but
scripts/seed-cut.sh did not, and its darwin-arm64 seed is
`go run ./scripts/xobuild`, a host build. The 0.47.0 darwin-arm64 seed
is separately compiled (1412 local symbols defined twice). The 0.44.0,
0.46.0, 0.48.0, 0.49.0, and 0.49.1 seeds and every darwin release
from 0.44.0 to 0.49.1 are whole program builds.

## Fix

- scripts/seed-cut.sh exports `XO_SEPCOMP=0`, as release.sh does.
- scripts/seed-pack.sh refuses a bin/xo that defines more than 100 local
  text symbols twice (`nm XO | awk '$2 ~ /^[tT]$/ {print $3}' | sort |
  uniq -d | wc -l`: a separately compiled xo about 1400, a whole program
  build 0); `XO_SEED_ALLOW_SEPCOMP=1` skips it.
- To compare code sizes or bytes of two xo builds, build both with
  `XO_SEPCOMP=0` (or caches off). `xo build --release` is never
  separately compiled.
