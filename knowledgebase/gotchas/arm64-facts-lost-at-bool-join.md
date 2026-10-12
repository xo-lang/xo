# arm64: a loop condition `a && b` proved nothing in the loop body

Status: Observed (2026-10-12), fixed

Symptom: `while i < n && i < m { x[i] ... y[i] }` kept both bounds
checks in its body on arm64 after decision 0184's bounds check removal,
while `while i < n { x[i] }` lost its check.

Cause: the SSA builder lowers `a && b` to a join block with one Bool
parameter, reached with `false` from the test of a and with the value
of b from the second test; the body is entered from that join. The
memo facts (branch bounds, `less` facts) were taken only from a single
predecessor ending in a branch on a compare, and the join's branch is
on its parameter, so neither `i < n` nor `i < m` reached the body.

Fix: `joinSource` (internal/arm64/opt.go, `join_source` in
selfhost/arm64/opt.xo) recognizes a block entered only from a join that
holds nothing but a branch on its one parameter, where every other
predecessor passes the constant that takes the other edge; the block
then inherits the memo of the one predecessor that can take its edge
and the facts of the Bool that predecessor passes. Golden bce_inline
(`common`, `skip_spaces`) covers `&&` and `||`.
