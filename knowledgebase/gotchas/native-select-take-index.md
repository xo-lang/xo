# A native receive arm after an `after` arm lost its value

Status: Observed (2026-10-09, found by TestCompileProperty, decision 0110)

Symptom: `select { after 1ms => ...  v = ch.recv() => got = v ?? -1 }`
with a value waiting printed -1 natively (`None`) and the value on the Go
backend.

Cause: `internal/ssa/build_conc.go` `selectExpr` took the received value
with `rt.select_take(sel, i)`, i the arm's position among all arms; the
runtime numbers only the channel arms (`after` is -2).

Fix: take it by the arm's runtime index (`rtIdx[i]`). Golden
`internal/llvm/testdata/golden/select_arm_order`.
