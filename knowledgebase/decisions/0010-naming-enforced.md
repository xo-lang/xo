# 0010: Naming conventions are compiler errors

Status: Proposed, pending benchmark (2026-10-07). See ../../bench/README.md

## Decision
`UpperCamel` types and variants, `snake_case` functions, variables, fields,
modules, `UPPER_SNAKE` constants. Violations are `XO0101`. Exports use `pub`.

## Consequences
- Renaming a field does not change its visibility (unlike Go capitalization).
- One canonical style for agents to produce.

## Evidence
- Paper test 2026-10-07 ([results](../../bench/results/paper-2026-10-07.md)): 0 violations; `ok err some none` contradict the rule and reserve common names.
- Paper test 2026-10-07 spec 0.2 ([results](../../bench/results/paper-2026-10-07-v0.2.md)): 0.2 run: 0 violations.
- Paper test 2026-10-07 spec 0.3 ([results](../../bench/results/paper-2026-10-07-v0.3.md)): 0.3 run: 0 violations.
