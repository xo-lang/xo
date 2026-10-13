# IOFlags matches runtime symbols by prefix

Status: Observed (2026-10-13, decision 0195, std/native/llrt/gen.xo)

Symptom: a C path's flag (`-DXO_GEN_C`) would turn on for every test
binary that generates a Bool, so their runtime would be compiled locally
instead of coming from the pack, though nothing calls the C path.

Cause: `build.IOFlags` (and xo1's `io_flags`) look for a symbol by
`callsSym`, which tests that the code contains a reference prefix (`bl
_`, `@`, ...) followed by the symbol's name, with nothing after it
checked: `xo_gen_int`, the C path's genInt, also matches `xo_gen_intn`,
which the default path still calls.

Fix: name a C path's functions so that none is a prefix of a symbol the
default path calls (`xo_gen_int` became `xo_gen_any_int`), or list only
symbols with that property; a prefix shared by symbols of the same C
path is harmless (`xo_gen_size` also matches `xo_gen_sized`).
TestIOFlagsDigestAndGlob checks `xo_gen_intn` turns on nothing.
