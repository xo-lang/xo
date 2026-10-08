# Sharing cheap constants across blocks costs registers and moves

Status: Observed (2026-10-08, internal/arm64, bench sieve golden)

Symptom: after per block memos started to inherit from the dominator,
sieve's code grew a callee saved register and a `mov x20, x22` before a
loop: the constant 0 that initializes two loop counters was made once and
copied into each counter.

Cause: a shared constant register is live from its first use to its
last, so it overlaps the loop counter it initializes and the two cannot
be coalesced; a fresh `mov x20, #0` per block coalesces with the counter
and costs the same one instruction as the copy.

Fix: `inherit` in `internal/arm64/opt.go` passes on only constants that
take more than one instruction (addresses, wide immediates, nonzero
floats); a constant one `mov` makes is made again in each block.
