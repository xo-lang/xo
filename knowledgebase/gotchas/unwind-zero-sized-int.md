# The zero value of an unwinding function was an Int where a Rune is returned

Status: Observed (2026-10-09, decision 0085)

Symptom: the SSA verifier rejected `prelude.bytes_decode_rune` (golden
`native_bytes`, `native_sql`): `tuple v38, v39: element 0 has the wrong
type`.

Cause: a function's unwind exit returns zero values (`zero` in
internal/ssa build_conc.go), and `zero` made every `KInt` with
`s.cint(0)`, the builtin `Int`. Sized integers and `Rune` are `KInt`
types of their own (decision 0081). Before fault unwinding, only
functions with cancellation checks had unwind exits, and none returned a
`Rune`.

Fix: `zero` makes a constant of the type itself. Code that builds a
value from a `Kind` must keep the `*Type`, not the kind's builtin.
