# A constant's name is not a match pattern

Status: Observed (2026-10-18)

## Symptom

`match t { R_SUBTRACTOR => 5, _ => t }` and `typ is RX_SIGNED | RX_SIGNED_1`,
with R_SUBTRACTOR and RX_SIGNED Int constants, fail to compile:
"error XO0302: variant pattern `R_SUBTRACTOR` does not fit a value of
type `Int`".

## Cause

In a pattern a bare name is a variant (or a binding), never a constant's
value; only literals match by value.

## Fix

Compare: `if t == R_SUBTRACTOR { ... }`, `typ == RX_SIGNED || typ ==
RX_SIGNED_1`. Literal patterns (`3 => 1`, `"PLT" => ...`) work.
