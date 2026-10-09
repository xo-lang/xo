# XO0305 asked to list 79 variants for a default arm on a large enum

Status: Observed (2026-10-08, `selfhost/lexer`, enum `Kind` with 88 variants).
Resolved (2026-10-08) by decision 0080.

Symptom (before 0080): `match k { Ident => "id" ... _ => "" }` on a token
kind enum declared in the same module was warning XO0305, whose message
listed every hidden variant (79 names on one line), and `xo check` was no
longer clean.

Cause: XO0305 (core 3.5) warned on any `_` arm that hides variants of an
enum of the same module, so that a new variant is a compile error at every
`match`. For "a few kinds are special, the rest are not" there was no way to
say "default" without the warning.

Fix: none needed now. A `_` arm that hides at least 8 variants and more
than half of the enum is a default and is not reported; the message lists
at most 8 names. Still reported: a `_` hiding a few variants of a large
enum, or most of a small one (2 of 3); list those variants.
