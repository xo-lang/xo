# 0256: inlining functions with landing pads

Status: Proposed (Xo Lang 2, 2026-10-17; supersedes, when accepted, the
"no landing pads of its own" rule of 0085's arm64 inliner).

Stub: the number is claimed; the context, decision, consequences, and
alternatives follow with the change. The aim: let the arm64 and amd64
inliner (compiler/arm64 inline.xo) copy a callee whose calls carry
landing pads (decision 0085), so that a generic like list.map, whose
closure call may fault, can be inlined and its closure call made direct
(hof 1.70 times LLVM on arm64, bench/results/amd64-vs-llvm-2026-10-17.md).
