# Go backend: a prelude enum's hand written methods need value receivers

Status: Observed (Xo Lang 1, 2026-11-10, decision 0586's `Ordering`)

**Symptom.** `go build` of a program on the Go backend fails with
`cannot call pointer method M_reverse on rt.Ordering`, and xo reports
"a code generator bug".

**Cause.** The generated code calls a prelude type's method on whatever
value the receiver expression gives, often a call result
(`rt.OrdCmp(a, b).M_reverse(tk)`), which Go cannot take the address of.
The pointer receivers std/rt uses for its handles (`*ByteBuffer`,
`*FaultErr`) work only because those receivers are variables.

**Fix.** Write the methods of a small value type (an enum without
payload) with value receivers, `func (o Ordering) M_reverse(*Task)`. The
signature check stdgen writes, `(*Ordering).M_reverse`, accepts a value
receiver too, since a pointer's method set includes the value methods.
