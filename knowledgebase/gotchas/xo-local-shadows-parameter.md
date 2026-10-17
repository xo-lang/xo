# Xo: a local binding may shadow a parameter of its function

Status: Observed (2026-10-11, compiler/arm64)

Symptom: the first run of the self hosted arm64 emitter faulted with
`index 0 out of range for List of length 0` while expanding an
instruction template: every instruction made by `plain` and `store` had
no uses.

Cause: a mechanical rename (the field and parameter `uses` became `us`,
since `uses` is a keyword) turned `fn plain(text: Str, uses: List[Int])
{ var us: List[Int] = []; for u in uses { us.push(...) } }` into a body
whose `var us` shadows the parameter `us`, so the loop walked the new
empty list. The checker accepts it: shadowing is an error only within
one block (core 3.1), and parameters are in the scope around the body.

Fix: give the local another name (`rs`). After a rename, search for a
`var` or `let` of a parameter's name in the same function.
