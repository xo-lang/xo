# 0038: No Map index reads; warnings for own enum wildcards and split critical sections

Status: Accepted (2026-10-08)

## Context
An independent second author planted 30 safety bugs into the Xo solutions in
forms Xo did not catch: `m[k]` faulting on a missing key (Xo's nil
dereference), `_` arms hiding cases, and check then act across two
`Mutex.with` sections (`bench/results/planted-xo-2026-10-07.md`).

## Decision
- XO0410 (error): reading a `Map` by index or changing an element in place
  through it. `m[k] = v` stays. New `Map.get_or_fault(k, reason)`.
- XO0305 (warning): `_` or catch all binding arm hiding variants of an enum
  declared in the same module.
- XO0804 (warning): a value read in one `Mutex.with` (or derived from it, or
  branched on) flows into a later `with` on the same mutex.

## Consequences
Second author: caught 21 -> 23 of 30, caught or warned 25 of 30. First author
29 of 30 (the miss is not a defect). Logic bug control unchanged (16/20). On
the 8 stage 2 solutions: one new error (task 06 reads a map by index) and one
warning (task 03). Golden programs that read maps by index were rewritten.
Wildcards on prelude enums such as `target.os` are not warned by design.
