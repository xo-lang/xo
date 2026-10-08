# Go handles are shared references, not Xo values

Status: Anticipated (2026-10-08, Go interop prototype)

Symptom (expected): `let a = url.parse(s)?; let b = a; b.set_path("/x")`
changes `a.path()` too, and a `url.Values` handle returned by `u.query()`
mutated with `set` is seen by every copy. Xo values never behave like this
(core 5).

Cause: decision 0041 maps mutable Go objects to opaque handles. A handle
of a Go struct holds a pointer (`struct{ P *T }`), and a handle of a
named map, slice, or interface holds the Go value, which is itself a
reference. Copying the Xo value copies the reference.

Fix: treat handles like capabilities (`Fs`, `Router`): one owner. The
binder already skips pointer receiver methods on value types such as
`uuid.UUID` (`UnmarshalText`, `Scan`), whose handles compare by value, so
those stay value like. Slices and maps that are not named types with
methods are copied at the boundary (`List`, `Map`), so they keep value
semantics. A copy on write handle is a listed next experiment in
`bench/results/go-interop-2026-10-08.md`.
