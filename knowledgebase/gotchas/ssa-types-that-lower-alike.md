# Checker types that lower alike were merged or split in the SSA builder

Status: Observed (2026-10-09, decision 0083)

Symptom: the first native build of std/sql (`std/native/sql.xo`)
reported XO1201 "converting `Result[native.sql.Db, sql.SqlErr]` to
`Result[sql.Db, sql.SqlErr]`" at every call of `sql.memory` and
`db.query`, and "converting `Result[Time?, sql.SqlErr]` to
`Result[Int?, sql.SqlErr]`" at a call of a generic helper with a `Time`.

Cause: two separate slips with one root. Lowered struct and enum types
are named by the checker type's text. (1) A std type with a native
implementation (`sql.Db`) lowers to the implementation's struct, but a
type built around it (`Result[sql.Db, E]`, from the std declaration at
the call) kept the std name, while the implementation returns
`Result[native.sql.Db, E]`: two enum types with one layout. (2) Generic
instances were keyed by the lowered names of their type arguments, so
`f[Time]` and `f[Int]` (both lowered to Int) shared one instance whose
parameter types came from whichever was declared first.

Fix: `builder.implNamed` names lowered types with every std type that
has an implementation replaced by it (in arguments too), and `declare`
keys instances by the checker types (with implementations substituted),
suffixing the name (`#2`) when two instances lower to one name. Expect
the same trap for any new lowering that maps two checker types to one
SSA type: name the SSA type from the mapped checker type.
