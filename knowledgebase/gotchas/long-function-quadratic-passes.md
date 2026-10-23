# Passes quadratic in a long function's blocks or checks

Status: Observed (2026-10-22)

Symptom: the linux/amd64 build of a generated program with one function
of 24,000 lines took 113 s with debug information and 63 s with `-s`
(XO_PROF: compiler/ssa remove_blocks 37% of the CPU, arm64 dv_field 28%).

Cause: two loops did whole function work per item. compiler/ssa rc.xo
merge_jumps renumbered every block, value and edge (remove_blocks) after
each merged block and restarted its scan, so a function of thousands of
landing blocks paid blocks times values (insert_rc 42 s). compiler/arm64
dwvars.xo dv_field looked at every fault stub for every variable and
built each stub entry's text, splitting the previous entry to extend a
run (emit 56 s against 8 s with `-s`).

Fix: merge_jumps merges every chain in one pass and renumbers once
(insert_rc 1.1 s). dv_field visits only the stubs inside a variable's
spans (binary search over the stubs, which the emitters record in
instruction order), keeps the open run as values and writes its text
once (emit 16 s). The build takes 31 s with debug information, 22 s with
`-s`; the output is byte identical. Look for a renumbering or a full
scan inside a per block or per check loop first when a long function's
build is slow; compiler/ssa build.xo read_var (Braun's construction,
11 s of the 24,000 line function) caches each definition in every block
of a single predecessor chain and is the next one.
