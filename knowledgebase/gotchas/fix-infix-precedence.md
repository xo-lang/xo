# A fix that writes an infix operator changes what the code means

Status: Observed (2026-11-10, the owner's word count on a Mac).

## Symptom

`xo check --fix` (the `.or(x)` habit, XO0601) rewrote
`m.get(w).or(0) + 1` to `m.get(w) ?? 0 + 1`. The program compiled, `xo vet`
was silent, and every word counted 1.

## Cause

`??` binds loosest of the binary operators (core 15), so the rewrite
parsed as `m.get(w) ?? (0 + 1)`: the `+ 1` moved into the default. The fix
replaced a postfix call, which binds tightest, with an infix form, and
looked at nothing around it.

## Fix

compiler/check habits.xo `infix_at(span, text)`: a fix that replaces
source with an infix expression parenthesizes it unless both neighbors
end an operand (start or end of line, `(`, `,`, `=`, `return`, ...). The
`.or` / `.unwrap_or` / `.or_else` rewrites and the `append` to
`xs + [...]` rewrite use it; any new fixer producing an infix form must.
`xo vet` XO0542 reports `x ?? lit <op> z`, the shape the old fix left
behind. Tests: cmd/xo TestOptionDefaultFixPrecedence (each operator
neighbor, and the fixed program's output) and TestVetCoalesceOperand.
