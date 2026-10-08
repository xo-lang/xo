# 0036: Readings from implementing spec 0.4

Status: Accepted (2026-10-07)

## Context
Implementing 0.4 in the parser and checker required choices the 0.4 text did
not settle (`bench/results/check-corpus-v0.4-spec.md`). They change what
`xo check` accepts, so they are recorded here and written into the spec.

## Decision
- Elided effect arguments: all or none per type; implicit effect variables in
  parameter and receiver types (counted as declared); inferred from the body in
  result and error types; implicit struct parameters in fields (core 6.2).
- Group tasks: origin visible means the error must convert like `s.spawn`;
  origin not visible means the task must not fail unless awaited. The escape
  rule covers values from that scope's `s.group()`; `os.tasks` may be stored
  anywhere (core 7.1).
- `testing.run_main` takes an effect variable (std 8.4).
- `->` on a following line continues a declaration (core 1.3).
- `is` is a contextual word; no bitwise compound assignments; an unused local
  `const` is XO0110; "stored again" for XO0520 includes later stores in the
  same loop.
