# XO0305 asks to list 79 variants for a default arm on a large enum

Status: Observed (2026-10-08, `selfhost/lexer`, enum `Kind` with 88 variants)

Symptom: `match k { Ident => "id" ... _ => "" }` on a token kind enum
declared in the same module is warning XO0305, whose message lists every
hidden variant (79 names on one line), and `xo check` is no longer clean.

Cause: XO0305 (core 3.5) warns on any `_` arm that hides variants of an
enum of the same module, so that a new variant is a compile error at every
`match`. For "a few kinds are special, the rest are not" there is no way to
say "default" without the warning.

Fix (workaround): a Bool test with an or pattern
(`k is Ident | Int | Float`), or a lookup list of `(Kind, Str)` tuples with
`find` (`short_name` in `selfhost/lexer/lexer_test.xo`). Proposed fix: gap
5 of `bench/results/selfhost-lexer-2026-10-08.md` (no warning when the arm
hides more than half the variants; cap the list in the message).
