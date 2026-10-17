# Parallel `xo build` runs share one build directory

Status: Observed (2026-10-13, decision 0206 stage 2 golden matrix).

Symptom: building the golden programs with xo1 from a script, 16 at
once (each in its own program directory, each with its own `-o`), gave
binaries that printed another program's output (`want 'select hello'
got 'deep calls unwound, finished 0'`), clang crashes ("no
preprocessable inputs"), and links missing runtime symbols
(`undefined reference to xo_fs_canceled`).

Cause: every build without `XO_BUILD_DIR` uses `<user cache>/xo/build`
(compiler/xo main.xo build_dir), and the intermediate files there
(prog.s, prog.ll, the objects) are named per build kind, not per
program, so concurrent builds overwrite each other.

Fix: give each concurrent build its own `XO_BUILD_DIR` (the scratch
golden matrix sets it per program). The Go tests do the same with
t.TempDir.
