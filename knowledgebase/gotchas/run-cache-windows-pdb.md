# A run cache hit wrote a windows .exe without its PDB

Status: Observed (2026-10-29, Xo Lang 2)

## Symptom

A `xo build --targets=windows/amd64` of an unchanged program left `prog.exe` but no
`prog.pdb` (or an older build's PDB, whose GUID the new image does not name); its
`--phase-times` showed only `phase run_cache`. A link identity check (off, miss and hit of
the PE prepare cache) compared nothing: every build after the first was a run cache hit.

## Cause

Whole-build reuse (decision 0360) stores one binary per entry and a hit copies it to each
output (wholebuild.xo whole_build_hit). The PE link also writes `NAME.pdb` beside the
`.exe` (ld.xo pe_link, decision 0344), which the entry did not hold.

## Fix

compiler/xo main.xo first stored no entry for such a build (bd4e544c8). Since then the
entry keeps the PDB as a side file (testcache.xo: a `side SUFFIX` line and the entry's
`.side<i>` copy; wholebuild.xo whole_build_hit writes it before the .exe), so an unchanged
windows build hits and gets the same PDB back; a PDB beside the .exe that the build did not
write (an older one, under -s) still stores no entry (cmd/xo TestWholeBuildWindowsPDB). A
link measurement or identity check sets `XO_WHOLE_BUILD=off` anyway, since the run cache is
keyed outside XO_BACKEND_CACHE_DIR (XO_BIN_CACHE).
