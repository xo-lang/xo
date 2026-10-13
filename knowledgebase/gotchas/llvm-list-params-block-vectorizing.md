# LLVM keeps Xo's word loops over several Lists scalar

Status: Observed (2026-10-13, decision 0207 stage 2); fixed for
Argon2id 2026-10-13 (decision 0211)

Symptom: the Argon2id in Xo (std/native/llrt/digest_tables.xo
`argon2_g`, `bs`, `t`, `q` three `var List[Int]` parameters) stays at
1.155 times the C's instructions on LLVM after the zero fill went to
`raw.zero_ints`, while arm64 reached 0.980
(bench/results/raw-memory-2026-10-13.md, stage 2). The disassembly of
the LLVM build of argon2_g has no vector instruction (each word is two
loads, an eor, and two stores); the C's `cr_argon2` has 115 (`.16b`,
`.2d`, `q` registers).

Cause: in the IR every List is a pointer to its object and the elements
are loads and stores through it; LLVM cannot prove that the stores to
`t` and `q` do not write the words of `bs` it loads next, nor that the
calls in between leave the lists' lengths alone, so its index checks
stay and its vectorizers leave the code alone. The C works on separate
local arrays, which it can.

Fix (decision 0211): the LLVM emitters give list accesses alias scopes
where value semantics prove two lists apart (a `var` list once made
unique, a new list, the borrowed parameters; noalias.xo), and argon2_g
checks the lengths of bs and q again after its permutation calls (see
llvm-length-after-call.md). Argon2id went to 1.043 of the C on LLVM and
is Xo by default (bench/results/llvm-noalias-2026-10-13.md). Two things
the scopes alone do not do: the SLP vectorizer leaves straight line
`r = a[i] ^ b[i]; q[i] = r; t[i] = r` as `ldp`, `eor`, `stp` (it does
the same for the equivalent C with `restrict`), and the scopes say
nothing about calls (`!noalias` on calls was measured and not kept:
llvm-call-noalias-spills.md).
