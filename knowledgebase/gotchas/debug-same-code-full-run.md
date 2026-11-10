# TestDebugInfoSameCode fails only in the full internal/arm64 run

Status: Observed (2026-11-09, lead's tiers 62 at f609f99c4 and 63 at
fdf4d71ef, gate.sh's arm64 step on the Mac)

## Symptom

10 of 642 goldens (closures, maps, structs, json_text, http_server,
fault_unwind, conc_scopes, chain_pads, value_semantics, str_ops_table)
report every program function of the build without debug information
(`main`, `cabi_c.main`, closures) as having no function with its code in
the build with it. `-run '^TestDebugInfoSameCode$'` alone passes at both
commits.

## What is known

- A pair whose builds differ in std's separate compilation (decision
  0388: std's store used in one, the whole program in the other) gives
  exactly these names (closures: 22 functions, `cabi_c.main`,
  `double.value`, `main.fn5`); two builds in one mode match, in any
  order of a fresh, shared or warm backend cache (2026-11-09, scripts in
  ~/xo-lead/notes/xo-lang-2/dbgsame).
- But 7 of the 10 are in scripts/docker-amd64-sample.txt, which
  xotool.Build always builds with XO_SEPCOMP=0, so for them both builds
  are whole program: a std variant miss does not explain them. The
  cause is open.

## What the test does now

On a mismatch it logs the first differing function in both builds, keeps
both binaries under $XO_TEST_KEEP_DIR when set, builds the pair again with
--phase-times (logging each build's std variant lines) and compares that
pair (logged, the first pair's mismatch fails the subtest). The
next full run with the mismatch leaves the evidence in its -v log.
