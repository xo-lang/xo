# 0537: std/timego, Go layout compatibility for time

Status: Accepted (owner, 2026-11-06: "Go with your recommendations", on the Lead's relay; drafted by Xo Lang 3)

## Context

The AWS port's azfile SAS code (aws feedback 5f3c3e5, 2026-11-06) had to hand write a scanner for Go's four SAS time layouts:
Go's `time.Parse` accepts what Xo's strict layouts (decision 0087, spec/std.md 13.3, "parsing accepts exactly the layout")
do not, and every SDK port that reads timestamps written with Go layouts hits the same gap. Go's rules include a one digit
`15` hour (`3:04PM`-style layouts), a fraction accepted after the seconds field even when the layout has none, `.0000000`
needing exactly that many digits, `_2` space padded days, `Jan`/`January`/`Mon`/`Monday` names and `MST` zone abbreviations.

The core time API stays as decided: layouts are named fields (`{year}-{month}`), letter patterns are habit errors (`XO0177`),
and nothing is guessed. Go compatibility is a separate concern.

## Decision

1. **A new module std/timego** (a compat module like Go's own `time` layout constants, not part of `std/time`):
   `fn parse(s: Str, go_layout: Str, z: Zone) -> Time ! ParseErr` and `fn format(t: Time, z: Zone, go_layout: Str) -> Str`,
   plus the Go layout constants (`RFC3339`, `RFC1123`, `RFC822`, `Kitchen`, `DateTime`, `DateOnly`, `TimeOnly`, ...), with
   Go's `time.Parse` and `time.Format` semantics: the reference layout `Mon Jan 2 15:04:05 MST 2006`, every element of Go's
   `nextStdChunk` (including `_2`, `__2`, `002`, `.000`, `,000`, `.999`, `Z07:00`, `-0700`, `-07`, `PM`/`pm`), Go's
   leniency rules (a fraction after `05` is accepted when the layout has none; a one digit field where Go allows it) and its
   error cases (`ParseError` text follows Go's `parsing time "x" as "y": cannot parse ...`).
2. The module is built on `std/time`'s `Time`, `Zone` and `ParseErr`: it parses with the Go rules and builds the same `Time`;
   it does not add a Go mode to `time.parse`. A Go layout can be translated to an Xo layout (`timego.to_layout(go_layout)
   -> Str?`) when an exact translation exists, so callers can move to the core API.
3. **Maturity** (decision 0461): Solid on all three axes. Go's own `time` format and parse tests are ported (the layout
   table and the `parseTests`/`formatTests`), a differential against Go's `time.Parse` and `time.Format` runs on a seeded
   corpus of layouts and strings (the oracle is a Go module under internal/stdtest, like protoref), the layout scanner is
   fuzzed, and a timing row compares it with Go's package.

## Consequences

- AWS and Azure SDK ports read Go layouts without a hand written scanner.
- Two layout languages exist; `timego` is documented as a porting aid, and `std/time`'s docs point to it from the habit
  error for Go style layouts.
- A std change log (std/changes/timego.md), a coverage row (infrastructure/std-coverage.md) and the new module's index line.

## Alternatives

- **A Go mode in `time.parse`** (`parse_go` on std/time): rejected by the owner; the core API stays per 0087.
- **Translate Go layouts at compile time only**: rejected; layouts from data (config, wire formats) are common in ports.
