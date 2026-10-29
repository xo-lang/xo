# Separate compilation stage 2: the edit checked against std's records (2026-10-28)

Decision 0388 stage 2 (Lang 1's design, ~/xo-lead/notes/lang1/design-0388-stage2.md,
built by xo-lang-6 with Lang 1 reviewing): a separately compiled build no
longer parses std to check the program. The stdx entry carries a header
(the fingerprint of std's sources, a stamp of every std file and directory:
size and time, the content's hash within 2 s of recording, a directory's
.xo names within 2 s) and the module closure (every std module with its
files, their interface hashes, content hashes and imports). When the
stamps hold, std's modules and files are made from the record without a
read; the records install without their trees; only the 23 of std's 239
files holding a generic object are parsed before install (the SSA builder
instantiates std generics transitively), the rest join on demand (a
constant's initializer, a lazy body, a missing default, a type's
declaration, a time layout's constant). A std body the facts did not make
extern that meets a pending tree makes the driver join every tree and
build the front end again (never a body-less build). Whole-program builds
and xo doc, def, refs, the LSP and test keep every tree.

Setup: the development Mac (darwin/arm64, 24 GB), main ada872969 against
this change, XO_STD_EXPORT=1 and XO_SEPCOMP=1 (store and facts from `xo dev
stdcache`) on both sides, a private backend cache, three warm builds then
10 edits (an appended function each), alternating main and the change;
--phase-times and XO_CACHE_STATS=1; `/usr/bin/time -l` wall. The machine
was shared (load 8 to 12).

| fib edit | std files parsed | check | wall |
|---|---|---|---|
| main | 102 | 32 to 37 ms | 0.10 to 0.11 s |
| stage 2 | 9 | 18 to 19 ms | 0.08 to 0.09 s |

| HTTP server edit (bench/perf/http, api.xo) | std files parsed | check | wall |
|---|---|---|---|
| main | 179 | 59 to 105 ms (59 to 64 in the quiet rounds) | 0.21 to 0.49 s (0.21 to 0.22 quiet) |
| stage 2 | 16 | 27 to 69 ms (27 quiet) | 0.16 to 0.31 s (0.16 to 0.17 quiet) |

Lang 1's oracle (2026-10-27) projected check 31 to 16 ms on fib. The
header check costs one stat per std file and directory (about 300) per
build.

Gates: compiler/check's tests (the header round trip, settled and
unsettled stamps, directory names, a file written after the build's start,
the closure's records), XO_STD_EXPORT=verify under separate compilation
and whole-program (0 differences), a program formatting with time.RFC3339
and one with a bad literal layout (its error kept), identity and the fixed
point: identity (scripts/identity ada872969 against this head, plain and with XO_STD_EXPORT=1 exported) compared 202, differing 0 both times; the fixed point bootstrap ok (s2 and s3 identical), chkcache and cross ok; TestStdTreesRebuild (the rebuild fallback) passes.

## What XO_STD_EXPORT=1 gains over today's default, and the default

Today's default (XO_STD_EXPORT unset) parses every std file the program
reaches on every build and declares std from the parses; the records
and this stage are opt in. With XO_STD_EXPORT=1 a separately compiled
edit skips the std parse and declare entirely: fib's check 33 to 18 ms,
the HTTP server's 60 to 27, and the stdx entry is one file per flags
and std tree. Turning it on by default needs its own default-change
gate: the full compiler/check and cmd/xo suites and the dev goldens
with it on, XO_STD_EXPORT=verify over the goldens, the fixed point, and
the first-build cost of writing the entry measured
(bench/results/first-build-2026-10-28.md's setup).

## Readiness for a default (2026-10-28, lead's list, each run as a bulk or measure job on main be05c22f0)

| check | result |
|---|---|
| cmd/xo full suite with XO_STD_EXPORT=1 | ok (251 s) |
| compiler/check's tests with XO_STD_EXPORT=1 | 37 passed |
| the native goldens (TestGoldenRun) with XO_STD_EXPORT=1 | ok (111 s) |
| the native goldens with XO_STD_EXPORT=verify | ok (75 s, 0 differences) |
| the fixed point with XO_STD_EXPORT=1 | bootstrap ok (s2 and s3 identical; xobuild 98 s) |
| a brand-new user's first build and the edit after it, plain against XO_STD_EXPORT=1 (fresh HOME each round, pairs interleaved, n=5, a measure window at load 5 to 23) | fib first build 1059, 1838, 2358, 2817, 2925 ms plain against 1311, 2001, 2829, 2680, 3003 (the rounds drift 3x; mean +8%); fib edit 232 to 481 against 258 to 449; HTTP first build 8771, 3768, 3897, 3906, 3847 against 4616, 3908, 3761, 3737, 3501; HTTP edit 702 to 815 against 644 to 822. Instructions retired (fib first build, fresh HOME, n=6, load 4): plain 19.55, 19.45, 19.54, 19.50, 19.55, 19.58 G against 19.54, 19.60, 19.53, 19.59, 19.67, 19.60 G: +0.3% (the export, the header's stamps and a 400 KB entry write); the check phase 58 to 400 ms plain against 62 to 309 with the record, no phase added. The +8% wall is the drift, not the record. |

The default on main c462dd4e1 (2026-10-29, a quiet measure window at load 4
to 7, std's variants built first with `xo dev stdcache --conc=0|1`, six fib
edits): every edit prints `cache std ... reused` twice and `cache stdx record
loaded (101 modules)`, `cache parse 9/9 reused`, check 23 ms on the first
edit then 18, 18, 17, 17, 17, heap 75 MB; with XO_STD_EXPORT=0 the same
edits take check 54 ms (std parsed again) then 32, heap 116 MB. The same
as the opt-in numbers above. A gate's "default edit" step that did not
build the variants first measured whole-program builds at 44 to 53 ms
(gotchas/sepcomp-default-check-needs-std-variants.md).

What the flip itself needs when it comes (a default change, CLAUDE.md): the touched packages' full suites and a self build (s2) with it on, in its gate; the stdx entry's first-build write measured above.
