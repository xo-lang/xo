# `xo fmt` on an existing selfhost or std file rewrites lines nobody touched

Status: Observed (2026-10-13, decision 0200)

Symptom: after editing compiler/arm64/opt.xo, compiler/xo/link.xo, and
std/native/prelude.xo, `xo fmt --check` lists them, and `xo fmt` joins
struct literals, parameter lists, and `for` lists that were wrapped by
hand far from the edit (a 20 line diff for a 3 line change).

Cause: those files are not kept in `xo fmt`'s layout: the formatter
keeps a bracketed list on one line unless the source breaks right after
the opening bracket (gotchas/fmt-comments-in-joined-lists-move.md), and
the hand wrapped lines break after an element instead. No test runs
`fmt --check` over compiler/ or std/native/.

Fix: do not run `xo fmt` on a whole existing file there; check only the
lines you wrote (or run it, then `git checkout` the file and reapply the
edit). New files can be formatted whole.
