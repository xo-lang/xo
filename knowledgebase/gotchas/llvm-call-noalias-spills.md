# `!noalias` on calls makes LLVM keep values across them, and spill

Status: Observed (2026-10-13, decision 0211, measured and not kept)

Symptom: giving the calls of a function `!noalias` of the alias scopes
of the lists no callee can reach (lists whose values never leave the
function's list operations) made the unrolled Argon2id slower on LLVM:
1.093 of the C against 1.044 without it, the disassembly of argon2_g
storing vector registers to the stack (`stp q0, q4, [sp]`) around the
permutation calls (bench/results/llvm-noalias-2026-10-13.md).

Cause: once LLVM knows the calls leave a list alone, GVN forwards the
words stored into it (or loaded from it) before the calls to the loads
after them, so 128 words stay live across 16 calls and are spilled and
reloaded, which costs more than loading them again. LLVM's load
forwarding does not weigh register pressure.

Fix: none kept; the emitters put no metadata on calls. The same
metadata made the xor parts written as plain loops vectorize (0.869 of
the C), so it may come back once the arm64 backend runs such loops as
well as the unrolled form (1.400 there today): measure both shapes on
both backends first.
