# The cc phase of a large build was mostly xo scanning its own assembly

Status: Observed (2026-10-13), fixed (2026-10-13, 0653188b).

Symptom: `xo build --backend=arm64 --phase-times selfhost/xo` on the
Mac spent 112.41 s in `phase cc`, but `cc -c` of the same 437 MB
assembly took 46.81 s and the link about a second more. A once a second
sampler of the xo process showed xo itself using 64 s of CPU during the
phase (bench/results/fast-builds-2026-10-13.md).

Cause: selfhost/xo/link.xo `io_flags`, which picks the C runtime's
`-D` flags, asked `calls_sym(code, sym)` for about 50 runtime names, each
a `contains` of the whole assembly with six prefixes (300 scans of 437
MB, mostly misses that read every byte), and it ran twice per build
(the link inputs, and the default linker's pack check). Invisible on
small programs, a minute on the compiler's own build.

Fix: `code_syms` collects, in one pass, the identifiers that start with
`xo_` after the six prefix characters (skipping the Xo functions'
`xo_f_` names), and `has_sym` asks that set. The flags are the same.
Any new check over the generated code should scan it once, not once per
name.
