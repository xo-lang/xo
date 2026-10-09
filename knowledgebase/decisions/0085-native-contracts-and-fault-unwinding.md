# 0085: native contracts and fault unwinding

Status: Accepted (2026-10-09). Contracts: `internal/ssa/build_contract.go`,
`xo_fault_detail` in `std/llrt/xo_rt.c`. Goldens (Go, LLVM, and arm64
compared): `internal/llvm/testdata/golden/contract_ensures`,
`contract_invariant`, `contract_with`, `contract_requires`.

## Context
The native backends (LLVM and arm64, decisions 0046 to 0084) lowered
`requires` but skipped `ensures` and struct invariants, so a program the
Go backend faults on printed a wrong result natively
(gotchas/native-ensures-unchecked.md), and hybrid builds (decision 0084)
had to exclude such functions.

## Decision
1. **Contracts are checked in SSA where the Go backend checks them**
   (core 9.1, `internal/gen` fn.go and decl.go), with the same messages
   and locations:
   - `requires` at the start of the body (`requires failed: <source>`
     at the condition, or `Err(else)`).
   - `ensures` after the body against `result` and the `old(...)`
     values, which are taken after the `requires` clauses; a function
     that can fail checks them only on success. A function with
     `ensures` (or a `var` method of a type with an invariant) becomes a
     wrapper that calls its body as `<name>.body`, so every way out of
     the body (end, `return`, `?`) is checked once, after its deferred
     code, as on the Go backend.
   - A struct invariant after a struct literal (defaults included) and
     `with` (at that expression), and after a `var` method on success
     (at the method's name): `invariant of <Type> failed: <source>`,
     evaluated by `xo.invariant[T]` (one function per struct instance).
   - A failed `requires` or `ensures` prints the names it reads with
     their Debug text on the next line (`xo_fault_detail`), as std/rt's
     Require and Ensure do.
2. **Hybrid builds no longer exclude** functions with `ensures` or
   values of a struct with an invariant (decision 0084, point 1).

## Consequences
- A function with `ensures` costs one extra call natively (the wrapper;
  LLVM inlines it, the arm64 inliner takes small bodies).
- The four goldens print the same on the Go, LLVM, and arm64 backends.

## Alternatives
- **Check `ensures` at every `return` in the builder**: each exit would
  need its own copy of the clauses, and deferred code runs after the
  return value is computed, so the check would have to sit between the
  defers and the return of every exit; the wrapper gets the order right
  in one place.
- **Report `ensures` as XO1201 (planned)**: correct but takes away a
  feature the Go backend has; the lowering was small.
