# `scope` cannot name a field, a parameter, or a variable

Status: Observed (2026-10-13, decision 0206 stage 3)

Symptom: std/native/llrt/dns.xo with a struct field `scope: Int` (an
IPv6 address's scope id, as Go's IPAddr has a Zone) and a local `let
scope = ...` gave `XO0160: expected a field name, found keyword
`scope``, then a cascade: `XO0403: missing field(s) `` in `Addr`
literal`, `XO0165: expected a pattern`, and `XO0510: use of moved value`
for a builder used after the broken line.

Cause: `scope` is a keyword (structured concurrency: `scope s { ... }`).
Unlike `when` (when-is-a-keyword.md) the first error names it, but the
cascade below it does not.

Fix: rename (`zone` for the field and parameter, `zi` for the local,
`sc` for the RFC 6724 attribute). Read the first error of a cascade
first.
