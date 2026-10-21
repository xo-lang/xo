# Fixed metadata id bases collide once a module is big enough

Status: Observed (2026-10-21).

Symptom: the LLVM build of compiler/xo (XO_LLVM_SPLIT=12
XO_LLVM_LTO=none) failed in clang with `prog.0.ll:95701:1: error:
Metadata id is already used` at `!1005701 = distinct !{!1005701,
!1005700}`. Every smaller program built.

Cause: compiler/llvm numbers each function's noalias scope nodes from
SCOPE_BASE * (fi + 1) (SCOPE_BASE 100) and the debug metadata from a
fixed 1000000. A module of 10,000 functions or more (xo crossed it)
puts function 10,000's scopes on the debug numbers. Nothing failed
before because no module was that large: a fixed base is a limit no
test reaches until a real program does.

Fix: the debug numbers start at max(1000000, SCOPE_BASE * (functions +
1)) (gen.xo; unit test compiler/llvm mdids_test.xo), so a module under
9,999 functions keeps its exact IR. When numbering ids from fixed bases,
derive each base from the sizes of the ranges below it, and test the
ranges' disjointness at the sizes where they would meet rather than
needing a program that large.
