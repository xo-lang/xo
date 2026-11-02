# A layout file is read at the module root, and a seed must understand it

Status: Observed (2026-11-01, xo-lang-2).

Symptom: a layout file checked in for xo changed nothing, then gave a strange function order
(`http1.text_of` first) and no gain, while `--layout=FILE` on the same content worked.

Cause: `xo build` reads `xo.layout` at the module root, the directory of xo.mod (compiler/), not
beside the main package (compiler/xo/). Then a new `target any` line was ignored by the seed that
builds the first stage: it applied the file's names to the store link (`layout_syms`), with no
target check, but not to the program's functions (`fn_order`).

Fix: compiler/xo.layout, with no target line, which every xo reads as any target. Check a layout
by the first symbols of `nm -n bin/xo`, not by the build's exit status. A format change to a file
the self build reads has to be understood by the seed too.
