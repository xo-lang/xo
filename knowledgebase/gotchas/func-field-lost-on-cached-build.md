# A new Func field that is a calling convention must reach every copy and shell

Status: Observed (2026-11-03, Xo Lang 1, decision 0465 step 2)

Symptom: with `Func.ret_view` added (decision 0465 step 2), a cold build
verified and a second, warm build of the same program into the same
backend cache failed under `XO_INLINE_STRICT=1` with `the inlined arm64
module does not verify (...: edge to bN passes vM, which is not held)`.
Without the strict flag it only fell back to building without inlining,
so xobuild's second hop failed its smoke build while every golden run
passed. Turning off any one pass or cache with `XO_PASSES=-x` hid it,
since that changes the keys and makes the build cold again.

Cause: a function the reference counting takes from its cache is not
decoded but stood in for by a shell (rccache.xo `lazy_shell`, decision
0283 stage B), built from the function before the counting ran. Its
`borrowed` was passed in, but `ret_view` was copied from the uncounted
function, where it is still -1. The verifier then read a callee that
returns a view as one that returns a counted result. The same field was
also missing from `Func.shell()` (ir.xo) and from extern shells
(stdfacts.xo `add_extern`, `mark_extern`).

Fix: a Func field that callers or the verifier read as part of the
calling convention (like `borrowed` and `ret_view`) is set in every place
a Func is built without its body: `Func.shell()`, `lazy_shell` (from the
counting's result, not from `f`), `add_extern` and `mark_extern`, and the
entry codec and keys of rccache.xo (with keyfunc_test.xo's reference
key). Test a change like it with a cold build and then a warm build into
one fresh `XO_BACKEND_CACHE_DIR` under `XO_INLINE_STRICT=1`; a cold build
alone never reads the shells.
