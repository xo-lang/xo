# A stripped program's traceback named every frame after the image header

Status: Observed and fixed 2026-10-25.

Symptom: building a program with `-s` (or stripping one after the link)
and letting it fault printed a traceback whose frames were all
`_mh_execute_header`, and more of them than the program had: one frame
in the unstripped build, five to eight in the stripped one. The fault's
own line (`fault: ... at main.xo:5:12`) was still right.

Cause: the Mach-O symbol table's `__mh_execute_header` is a defined
symbol at the image base, inside `__TEXT,__text`. The traceback reader
(std/native/llrt/prof.xo `symbols`) takes each frame's nearest symbol
below its pc, and with the function symbols gone that header is the
nearest symbol for every pc. Each frame then printed a name, so the
"stop at main" rule never fired either.

Fix: `macho_symbols` skips `_mh_execute_header`, and `trace_text_at`
prints `traceback: (no function symbols: built with -s, or stripped)`
when no frame's pc is covered by a symbol, instead of a list of frames
nothing names. A `--release` link now keeps the `__TEXT,__text` local
symbols (the function names) and drops the other local symbols
(compiler/macho `LinkOpts.release`), so a release build still names its
frames; only `-s` gives them up.

Test: internal/arm64 TestTracebackNoSymbols (a `-s` build) and
TestReleaseSymbols (a release build names its frames; no LSDA names on
darwin).
