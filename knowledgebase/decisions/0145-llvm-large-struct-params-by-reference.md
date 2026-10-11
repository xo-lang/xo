# 0145: LLVM backend passes struct parameters of 256 bytes or more by reference

Status: Accepted (2026-10-10). Implemented in `internal/llvm/fromssa.go`
(`byRefOf`, `keptResults`, `fullResLL`, `sfn.slot`, `loadRef`,
`storeRef`; the parameter list and entry loads in `fn`, the call in
`call`, the write back in `OpReturn`). Test: golden program
`internal/llvm/testdata/golden/byref_struct` (TestGolden: the same
output as the Go backend, no leak). Evidence:
`bench/results/selfhost-checker-2026-10-10.md` (stage 8, LLVM profile).

## Context

The SSA passes every value as a value: a `var` parameter goes in and
comes back as an extra result. The LLVM lowering kept that shape, so a
struct parameter was a first class aggregate argument. LLVM splits such
an argument into one argument per field (the first eight in registers,
the rest on the stack) and returns the updated value through memory
field by field; the callee copies every field into its frame. The self
hosted checker passes its `Checker` (53 fields, about 1 KB) to almost
every function as a `var` receiver: a profile of the LLVM build (macOS
`sample`, 2026-10-10) showed no function above 4% of the time, with the
time spread over the prologues and epilogues that move those fields.

## Decision

- A parameter of a direct call is passed by reference when its type is
  a struct of 256 bytes or more (`byRefMin`) and the function is not
  called through a function value (closure bodies, wrappers, task
  entries, and functions whose address is taken keep the plain
  convention, so every caller agrees).
- The callee takes `ptr noalias` (a `var` parameter) or `ptr noalias
  readonly` (any other) and loads the value at entry. A `var`
  parameter's updated value is stored back through the pointer at every
  return (the unwind exit included, which returns zero values as
  before); its result is no longer part of the LLVM return value.
- The caller stores the argument into memory and passes its address:
  per struct type, the function's own `var` parameter of that type
  (which forwards the caller's memory down a chain of `var` calls) or
  one alloca; a call with several such arguments of one type takes
  further allocas. After the call it loads the `var` parameters back
  and rebuilds the results the SSA call has, so the SSA is unchanged.
- Loads and stores are of the whole aggregate. Field by field loads and
  stores (so LLVM could drop unused fields) measured no faster: the
  unwind paths drop every field of the value loaded at entry, which
  keeps all of them live across the calls.

## Consequences

- The LLVM build of the self hosted checker is about 15% faster on the
  selfhost input (5,348 to 4,549 ms for 20 runs); output is unchanged.
- The arm64 backend keeps the field by field convention (its own
  calling convention; not changed here).
- Memory of a by reference argument holds the value only during the
  call; nothing reads it afterwards except the `var` write back, so
  sharing one alloca per type between calls is safe.

## Alternatives

- **Enums by reference too** (done in decision 0147, 2026-10-10, with
  payload reads from the reference and values in stable memory passed
  by address: 5% faster). `Expr` (352 bytes), `Pattern` (768) and
  `Stmt` (1,336) are as large; passing them by reference measured 2 to
  3% slower, since a payload read stores the enum into an alloca
  anyway. Next experiment: read payloads straight from the reference
  (a `getelementptr` into the parameter's memory) and pass enums by
  reference again.
- **Keep the value in memory for its whole life** (field reads load
  from the reference at the point of use, the unwind path reloads
  instead of keeping entry values): removes the remaining copies but
  needs the SSA's liveness to know that no other value of the type was
  stored into the same memory meanwhile. The next step if the checker
  is still slower than the Go checker by more than 1.5 times.
- **byval**: still copies at every call.
