# The output file's name is in the program's ad hoc signature

Observed 2026-10-24. Symptom: two builds of one program with one xo, each with
its own XO_BACKEND_CACHE_DIR, differ by exactly one byte (`cmp -l`), inside the
code signature at the end of __LINKEDIT; it looked like a reproducibility bug
(an earlier note here and in 0363 said builds differ between cache
directories).

Cause: the darwin ad hoc signature's identifier is the output file's base name
(`codesign -dv` shows `Identifier=plain-1`), and the two outputs were named
`plain-1` and `plain-2`. Nothing else differs: bench/perf/http/xo, plain and
`--release`, built into two private cache directories under the same output
name, are equal byte for byte (checked with a main xo, 3f8eabb38).

Fix / how to compare: name the outputs alike (put them in two directories as
`.../1/prog` and `.../2/prog`), then `cmp`. Only builds of one xo are
comparable; an xo that differs in the runtime changes the code of every
program that links it.
