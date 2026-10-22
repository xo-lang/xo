# Constant time checks: an inlined allocation's select fails a secret function

Status: Observed (2026-10-22, bigmod.exp; earlier mlkem.decaps_key)

Symptom: TestConstantTimeLLVM (full) fails with "bigmod.exp: a conditional
select after -O2"; the csel is `cmp xN, #4; csel` right after
`bl _x.cabi.obj_rl_new_n`, the capacity word of a new list.

Cause: a checked function that allocates gets the allocator's capacity
rule (llrt obj.rl_new_n: at least 4) inlined by clang -O2; that select
is on a public length, but the checks (noSelect) cannot tell it from one
on a secret. The full test runs only in the full tier, so it can stay
broken for a day unseen.

Fix: keep the secret work in a function that allocates nothing and
reads and writes its buffers by address (mlkem.implicit_reject out of
decaps_key; bigmod.exp_into out of exp), and point both checks
(TestConstantTimeLLVM's ctChecks, test/codegen/consttime's `// func`)
at it. Bisecting such a failure: verify GOOD first (scripts/bisect.sh
does since 31e5fa9e1); the commit that put the select in bigmod.exp is
named here once the bisect from 4ae585f11 (where the check was added and
passed) ends.
