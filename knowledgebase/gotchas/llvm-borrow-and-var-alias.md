# A list passed both as `var` and by value aliased in the LLVM backend

Status: Observed (2026-10-08, golden test `var_alias`, found in review)

Symptom (before the fix): `append_all(xs, xs)` with
`fn append_all(var dst: List[Int], src: List[Int])` let `src.len()`
inside the callee see the elements pushed through `dst`, breaking value
semantics. The checker accepts the call.

Cause: the LLVM backend passes by value arguments as borrows (no retain,
the list buffer is shared without marking it), relying on the callee not
being able to mutate them. With a `var` parameter in the same call the
callee can mutate the same buffer through the other parameter.

Fix: when a call has any `var` parameter or receiver, its by value
arguments are owned (retained), so a write through the `var` one copies
first (`fn.args`, `fn.valueCall` in internal/llvm/call.go).
