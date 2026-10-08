# Numeric conversion is `Float(n)`, not `Float.from(n)`

Status: Observed (2026-10-08, writing `examples/notes-service`)

Symptom: `Duration.from_secs(Float.from(n.ttl_secs))` fails with
`XO0601: Float has no associated function or constant from`, with no hint.

Cause: numeric conversions are calls on the type name (std 2.1:
`Float(x: Int)`, `Int(x: Float)`, `Int32(x)`), and `from` is only a
contextual word for enum variants. Rust habit (`f64::from`) leads to the
wrong form.

Fix: write `Float(n)`. For durations from whole seconds, `1s * n` is shorter
and avoids floats. A hint in XO0601 is proposed in
`bench/results/real-app-2026-10-08.md` (gap 8).
