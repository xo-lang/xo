# Every golden binary differs after a std edit, with no code change

Status: Observed (2026-11-15, Xo Lang 2, decision 0657's experiment).

Symptom: built by the parent's and the child's xo, all 742 golden
binaries of a change (Lang 3's openenum) differed, and stayed different
with `--binfo-stable`; the change touched no golden's code.

Cause: three things change a binary without changing its code.

1. The build-info id (compiler/xo/binfo.xo build_info): 24 hex written
   twice into .rodata (the `{"id":"` record and the `xo VERSION, BACKEND,
   TARGET, id ID` line). It hashes every source file's digest (every
   program reads std's prelude closure) and the embedded runtime's files
   (std/rt/*.go, *.s). Any std or std/rt edit changes it in every binary.
   `--binfo-stable` drops the source digests but not the runtime's sums.
2. The std path: it is in the debug information and strings, so two
   checkouts' builds differ in every binary (and in size).
3. C-linked goldens (`use c`): their C step records a path that differs
   from build to build (.debug_line and .rodata at equal sizes).

On darwin a fourth: the code signature and UUID take the output path
(darwin-binary-identity-output-name.md).

Fix: compare with the id masked (scripts/golden-bin-select.py), build
both sides with std copied to one path and to one output path, and treat
C-linked goldens as always changed. With that, E differed in 14 of 743
and openenum in the 8 C goldens only. Same xo twice, caches on or off,
and a moved golden directory were byte-identical.
