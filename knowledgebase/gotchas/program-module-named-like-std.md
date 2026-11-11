# A program package named like a std module it does not import shared std's symbols

Status: Observed (2026-11-11, hourly at b58a9318b, internal/arm64
TestDebugInfoSameCode/landing_memo); fixed 2026-11-11

Symptom: a separately compiled native build of the golden landing_memo
(a program with its own package `sse`, a reduced copy of std/sse) failed
to link: `xo: link: duplicate symbol _xo_f_sse_2eParser_2efeed in
prog-darwin.22.o and .../w-.../prog-darwin.0.o`. It began when std/sse
landed (ADR 0603 stage 1); the golden had passed before.

Cause: SSA names a module's functions by its default name (`sse.`)
unless another module loaded in the same build has that name
(ssa path_named_mods, mod_prefix: then a program module is named by its
path). The program did not import std/sse, so std's sse was not loaded
and the program's package kept the bare prefix. But std's object
(decision 0388) holds every std module whether a program imports it or
not, and the link pulled std's part holding std's `sse.Parser.feed` for
other functions in it: two strong definitions of one symbol. Whether the
link pulls that part depends on how std's object is split, so a small
program with the same names may link (and then a std caller of std's
function could bind to the program's).

Fix: the checker lists the default names of every module std's object
may hold (check std_object_names: std/M.xo, std/native/M.xo and its
directory modules, std/native/llrt/M.xo, as Checker.std_obj_names), and
path_named_mods names a program module with one of them by its path
(`github.com/.../sse.Parser.feed`), loaded or not. Regression:
internal/arm64 TestProgramModuleNamedLikeStd checks the golden
pkg_named_std's symbols (its own `sse`), since the link alone passes or
fails by how std is split.

Avoid: a symbol name that must not clash with std's has to account for
std modules the build never loads; check what std's object holds, not
what the checker loaded.
