# A fresh xo's first builds are whole-program: a check of the separate-compilation default must build std's variants first

Status: Observed (2026-10-29, xo-lang-6, the XO_STD_EXPORT default's gate).

## Symptom

A gate step meant to show the std records in use ("default edit" builds
of fib with `XO_CACHE_STATS=1`) printed `cache parse 102/102 reused` and
a check phase, but no `cache std` or `cache stdx` line, and the same
numbers with `XO_STD_EXPORT=0`: the record never ran. A quiet
measurement on main showed the same 28 ms check with and without the
switch.

## Cause

std's variants (the store and facts behind separate compilation) are
keyed on xo's identity, so a freshly built xo has none. Its first
default build starts them in the background (about 10 s each on the
Mac, `XO_SEPCOMP_BG`) and builds the whole program meanwhile; edits made
within that window are whole-program builds too, served by the parse
cache. Nothing in the output says so unless the `cache std` line is
required.

## Fix

Before any check of the separate-compilation default (and so of the
std records), build the variants: `xo dev stdcache --conc=0` and
`--conc=1` with the same xo and std, as scripts/docker-sepcomp-sample.sh
and the Docker recipes do. Then require the lines: `cache std <key>
conc=false reused` (separate compilation took std's variants) and
`cache stdx record loaded` (the record was used) on an edit; a step
that prints neither has measured a whole-program build.

## Rule

A proof that a default path ran must assert the line that says it ran,
not the absence of a failure. The off switch's check ("0 stdx lines")
passes trivially when the on side printed none either.

## Also observed: `xo test` and XO_SEPCOMP_BG=0 (2026-10-30, xo-lang-4)

scripts/release-vs-go.sh's test pairs set `XO_SEPCOMP_BG=0` so no
background build races the measured rounds; that starts no variant
build at all (`std variant: miss, the whole program is built
(XO_SEPCOMP_BG=0)` under `--phase-times`), so every `xo test
--std-module` edit round built the whole program: base64's edit read
0.40 s and 538 MB (1.267 of go test's wall) against 0.27 s and 320 MB
from std's test variant. Build the test variants first, `xo dev
stdcache --test --conc=0` and `--conc=1` (4f6caee06), and require
`cache std <key> conc=... test=1 reused` in a check.
