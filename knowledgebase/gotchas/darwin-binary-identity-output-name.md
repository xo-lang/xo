# Two darwin builds differ by their output names

Status: Observed (2026-10-21, Xo Lang 2, decision 0274 step 2's identity
check).

Symptom: a program built twice by the same xo to `-o det1` and `-o det2`
differs, and on darwin only; it looks like darwin builds are not
deterministic.

Cause: the ad hoc code signature (compiler/macho's signer) names the
binary by its output file name, which is in the signature's code
directory: one byte differed ('1' against '2'). A second trap in the same
check: DWARF carries the std directory's path (bin/../std of each xo), so
two xo binaries of different checkouts give binaries that differ from
that string on, every offset after it shifted.

Fix: compare builds made to the same output file name (separate
directories), and give both xo binaries the same std (`XO_STD`). Then
darwin and linux builds are byte for byte the same build to build.

windows/amd64 (2026-10-22): a PE file holds its PDB's full path in the
CodeView record (RSDS), and the record's GUID and its debug directory
entry's TimeDateStamp follow from it, so two builds to a/prog and b/prog
differ in 21 bytes even with one xo. scripts/identity masks the record,
that time stamp and the CheckSum (maskPE); it also names a windows output prog.exe, as xo build does for
`-o prog` since d320967b1 (before, it looked for prog and stopped).

windows/amd64 again (2026-10-27, Windows CI run 40525427807): with
--trimpath (the default) the record holds only the PDB's base name, but
two builds to `-o edit` and `-o off` still differ, in three places: the
CodeView record in .rdata (the name, so the GUID and stamp hashed from the
image), .rdata's VirtualSize in the section table (the name's length),
and .rsrc (the version resource names the file). A check that compared a
cached edit against a cache-off build that way read "differ" on a correct
build; run 40532204416 built both as prog in two directories and every
section, the headers and the PDB were equal. Name both builds the same
(scripts/identity's PE mask covers the record, its stamp and the
CheckSum, not .rsrc).
