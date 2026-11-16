# A std type's `derive Json(via:)` was ignored by importers

Status: Observed (Xo Lang 3, 2026-11-16, task #89)

## Symptom

std/big's BigRat declared `derive Json(via: Str, to: rat_marshal, from:
parse_rat)`. `xo test --std-module big` passed, but a program importing
std/big encoded a BigRat as its fields (`{"a":{"key":[...]}}`) on arm64
and LLVM, and the Go backend's generated std/rt code called
`F_rat_marshal`, which does not exist (the build failed).

## Cause

Three gaps, each hidden by the module test (which checks std/big as a
root module):

- A build does not check a std module's bodies (check/dump.xo skips
  check_module for them), and check_module was the only caller of
  check_json_via, so `json_via` stayed unset for importers.
- ssa declared the `to` and `from` functions themselves: a std
  declaration has no body (XO1201); its body is std/native's (impl_of).
- gen named them with func_go_name (`F_name`), but a std declaration's
  Go function is `<Prefix>_name` in package rt.

## Fix

check_std_json_via runs the via pass for skipped std modules,
Builder.via_fn and Gen.via_go_name map to the implementation. Golden
big_json covers an imported std via type on every backend.
