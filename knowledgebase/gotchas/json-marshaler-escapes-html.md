# A custom MarshalJSON escapes HTML even under SetEscapeHTML(false)

Status: Observed (2026-10-10, stage 7 of the self hosted checker)

Symptom: the full section of `xo typedump` printed `=>` as `=>` and
`<` as `<` in messages, notes, and fixes, while the brief diags
section, written with the same `json.Encoder` and `SetEscapeHTML(false)`,
printed them as is; the Xo checker's first full lines differed only
there.

Cause: `diag.Diagnostic` has a `MarshalJSON` that calls `json.Marshal`,
which always escapes `<`, `>`, and `&`; the encoder's setting applies to
what it encodes itself, not to the bytes a Marshaler returns.

Fix: the Xo side escapes the three characters in the full section only
(`json_html`, compiler/check/dump.xo), as `xo check --json` prints them.
Compare a JSON output with the encoder path that really produced it.
