# LLVM checks list indexes again after any call

Status: Observed (2026-10-13, decision 0211, Argon2id's argon2_g)

Symptom: a function checks `o + 127 < bs.len()` once at entry, and LLVM
drops the checks of `bs[o]` to `bs[o + 127]` before a call but keeps
every one after it (`add`, `cmp`, `b.hs` a word in the final xor of
argon2_g, after the 16 calls of blamka_row and blamka_column), even
though bs is not passed to the calls and its SSA value is unchanged.

Cause: each index check loads the list's length again, and LLVM must
assume a call may write any memory, so the length after the call is a
new value the entry guard says nothing about. The alias scopes of
decision 0211 cover loads and stores, not calls. On arm64 the cause is
different (a `var` list the call takes comes back with an unknown
length: arm64-length-after-var-list-call.md), the fix the same.

Fix: check the length once more after the calls in the source
(argon2_g: `if 127 >= t.len() || o + 127 >= bs.len() || 127 >= q.len()`,
two to six instructions a call): LLVM then has one length to prove the
indexes against, drops the checks, and vectorizes the loop. Telling
LLVM that the call cannot reach the list (`!noalias` on the call) does
it too but costs more elsewhere (llvm-call-noalias-spills.md).
