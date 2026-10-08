# Numeric conversion is `Float(n)`, not `Float.from(n)`

Status: Observed (2026-10-08, writing `examples/notes-service`)

Symptom: `Duration.from_secs(Float.from(n.ttl_secs))` fails with
`XO0601: Float has no associated function or constant from`, with no hint.

Cause: numeric conversions are calls on the type name (std 2.1:
`Float(x: Int)`, `Int(x: Float)`, `Int32(x)`), and `from` is only a
contextual word for enum variants. Rust habit (`f64::from`) leads to the
wrong form.

Fix: write `Float(n)`. For durations from whole seconds, use
`Duration.from_secs_int(n)` or `1s * n` (no float rounding).

Since 2026-10-08 (decision 0060) XO0601 for an unknown associated function
of a numeric type called with one numeric argument carries the note
``convert with `Float(x)` `` and a first fix that rewrites `Float.from(n)` to
`Float(n)`, so `xo fix` repairs it. A non numeric argument
(`Int.from("3")`) gets no hint: that is `Int.parse(s)`.
