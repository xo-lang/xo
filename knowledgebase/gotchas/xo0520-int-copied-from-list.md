# XO0520 fires for an Int stored into or read from a List and then changed

Status: Observed (2026-10-13, decision 0193 step 4, std/native/llrt/jsontext.xo)

## Symptom

Runtime code in Xo that keeps a cursor in a `var` fails to check:

- `tape.push(p)` and later `p = e`: `error XO0520: mutation of p is
  never written back`, note "`p` was stored into a collection here; the
  stored copy does not see later changes";
- `var p = tape[i + 1]` and later `p += 1`: the same error, note "`p` is
  a copy of a collection element taken here".

Both are plain Ints; nothing is lost.

## Cause

The lint (core 5.2) tracks values copied into or out of a collection and
warns when the copy is mutated, whatever the type, so an Int position
counts like a struct.

## Fix

Push or read through a `let`: `let at = p; tape.push(at)`, or
`let lo = tape[i + 1]` then `var p = if neg { lo + 1 } else { lo }`
(jsontext.xo's int_ok checks so); or pass the element as a parameter to
a helper. Costs nothing in the generated code.
