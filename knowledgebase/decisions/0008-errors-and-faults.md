# 0008: Typed errors, no nil, faults separate from errors

Status: Accepted (2026-10-07)

## Decision
- `-> T ! E` declares failure. `!` alone means `! Error`.
- `?` propagates and appends a trace frame. Unhandled results are compile errors.
- No nil: optionals are `T?`.
- Typed to typed error conversion only via `from` enum variants.
- Bugs (overflow, bounds, contracts) are faults: they terminate the task, not
  error values. `std/task.isolate` converts a fault to a value at boundaries.

## Alternatives
- Go `error` everywhere: easy to ignore, no exhaustiveness.
- Exceptions: hidden control flow.
