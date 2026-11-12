# A field passed owned while its aggregate lives is retained

Status: Observed (2026-11-12, decision 0613 step 5 stage 4)

**Symptom.** A runtime uniqueness test (a count of 1) on a value the
caller passes owned is never true: every `var self` call through an
interface value took the copy path, 100,000 calls in 5.6 s (quadratic),
though nothing else held the value.

**Cause.** The caller passed `Field(v, xo.show)` owned while v, the
interface value read from the place, was still an owned root: the count
pass retained the field for the call and released v after it, so the
callee saw a count of 2. Binding the callee's function value to that
same field (borrowed) while also passing it owned did the same.

**Fix.** Pass the whole aggregate owned as its last use (here the
interface value itself, a parameter type the slot's result holds, so
rc.xo fv_arg_owned makes it owned), read any other words out of it first,
and do not borrow from it in the same call (the slot's function value
binds the code to a closure with no environment, xo.noenv). Check with
the assembly (`xo dev dump asm`): no count increment before the call.
