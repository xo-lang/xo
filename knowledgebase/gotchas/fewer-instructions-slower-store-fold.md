# Fewer instructions, slower: arm64 stores folded to base + immediate

Status: Observed 2026-10-23 (change parked, not landed).

Symptom: putting rt.u64_mul_hi in inline.xo's inline_intrinsic let
merge_straight join bigmod.mac_row's 4-word round into one block, and
addr_fold then put the round's stores at the loads' base with
immediates (`str x9, [x12, #8]`) instead of register offsets
(`str x9, [x5, x4]`, with the offsets added). rsa sign4096 instructions
0.940 against main, every gate passing, but in a quiet window
(`xo bench --ab`, 30 rounds, spread about 1%) wall 1.123 and cpu 1.095
worse. A variant with the merged blocks but stores left at register
offsets: instructions 1.006 and wall 0.999 against main; the folded
build against that variant wall 1.13. The fold was both the whole
instruction saving and the whole slowdown on the M-series Mac. Merging
alone gained nothing and cost P-256 +0.9% and X25519 +1.3% (field
products taking x19 and a frame).

Cause: not pinned. mac_row is called on destination windows shifted
one word per row (mul_at, redc_at), so each row's loads read words the
previous row just stored; the store addressing change likely made those
loads wait on the stores (store to load forwarding or memory
disambiguation). LLVM's version pairs the stores (two stp a round).

Fix / how to avoid: an instruction count (callgrind, measure.sh) never
proves a speedup alone (CLAUDE.md, agent-tools.md "Measuring"): every
codegen change claiming one runs one `xo bench --ab` in a quiet window,
and a drop in instructions with more cycles is a reason not to land.
The patch and notes: ~/xo-lead/notes/Xo lang 4/parked/
mulhi-inline-intrinsic.patch; the next attempt is stp pairing.

Observed again (2026-10-24, Xo Lang 1), the stp pairing tried: mac_row's
round rewritten in LLVM's shape (row addresses formed once a round, ldp
loads, each pair of results stored with one stp after its second word,
the same words and carries; refs/recover/2026-10-24/macpair). The native
code then matched LLVM's round, and RSA sign2048 was instructions 0.923
with wall 0.990, but sign4096 instructions 0.904 with wall 1.032, worse
(quiet --ab, 30 rounds). Not landed. Three different placements of
mac_row's stores have now cut instructions and lost on the wall at 4096
bits; LLVM's faster wall is likely not mac_row's shape. Next: compare
redc_at's and square_at's hot loops on the two backends.
