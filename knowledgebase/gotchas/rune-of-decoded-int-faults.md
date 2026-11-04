# `Rune(n)` on a decoded integer faults on a surrogate: a denial of service

Status: Observed (2026-11-04, the owner's v0.38.1 evaluation, b27).

Symptom: `idna.to_ascii("xn--ib9b")` stopped the process with `55296 is not
a Unicode scalar value, so it is not a Rune`; so did the regexp pattern
`\x{D800}`. One request carrying such a hostname stopped a service.

Cause: `Rune(n)` faults for a surrogate (0xD800 to 0xDFFF) and above
0x10FFFF. Punycode's decoder checked only `0..0x10FFFF`; regexp's literal
prefix converted a parsed escape directly.

Fix: convert input derived integers with `Rune.try(n)` (and the Go
reference's replacement, U+FFFD, where Go's `string(rune)` yields one), or
test for the surrogate range first. Audit (2026-11-04): every other
`Rune(` in std/native converts a table value, a decoded UTF-8 rune, or is
guarded (charset, html, jsontext, asn1, xml, toml use `Rune.try` or an
explicit range test). Probe a new decoder with `\u{D800}`, `&#xD800;` and
`xn--ib9b` style inputs.
