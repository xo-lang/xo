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
