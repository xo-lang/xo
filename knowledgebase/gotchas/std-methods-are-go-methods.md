# A std method on a std struct or enum is a Go method in rt

Status: Observed (2026-10-09, std/time)

Symptom: a new std module builds its runtime, but the generated program
fails in `go build` with `v_d.M_add_months undefined (type rt.Time_Date
has no field or method M_add_months)`.

Cause: `internal/gen` calls a method declared in a std file as a Go
method of the receiver's runtime type (`d.M_add_months(tk, n)`) whenever
`methodIsGoMethod` holds: the receiver is a std struct, enum, or opaque
handle outside the prelude scalars, and the method has no type
parameters of its own. Only statics (`Time_Date_parse`), module
functions (`Time_zone`), methods on prelude scalars, `Duration`, `Time`,
`Bytes`, `Rune`, and generic methods are package functions named
`<Prefix><Type>_<name>`.

Fix: write `func (d Time_Date) M_add_months(_ *Task, n int64) Time_Date`
in the runtime. Check `methodIsGoMethod` in `internal/gen/decl.go` when
unsure, or build a sample program and read the Go error.
