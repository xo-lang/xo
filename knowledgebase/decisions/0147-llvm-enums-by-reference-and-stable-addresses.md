# 0147: LLVM backend passes large enums by reference, and values in stable memory by address

Status: Accepted (2026-10-10). Implemented in `internal/llvm/fromssa.go`
(`byRefOf` takes enums, `sfn.addr`, `rootBorrowed`, `isAgg`; `OpTag`,
`OpPayload`, `OpField`, `OpUnbox`, `list.index`, and the arguments of
`call`). Test: golden program `internal/llvm/testdata/golden/byref_enum`
(TestGolden, TestGoldenShared: the Go backend's output, no leak).
Evidence: `bench/results/selfhost-checker-2026-10-10.md` (LLVM
experiments). Extends decision 0145.

## Context

Decision 0145 passes struct parameters of 256 bytes or more by
reference. Passing enums the same way measured 2 to 3% slower then: a
payload read stored the whole enum into an alloca to read one field, so
the copy moved from the call into every match. The self hosted
checker's `Expr` (352 bytes) and `Stmt` (1,336 bytes) are matched and
passed down on every node, and every call that passes a sub expression
copied it into a slot first, though it already sat in memory the
caller could not change (its own parameter's memory, a box, a list
element).

## Decision

- Enums of 256 bytes or more are passed by reference like structs
  (same rules: direct calls only, `var` written back).
- A value is *in stable memory* when nothing can write that memory while
  the function runs:
  - a parameter passed by reference that is not `var` (the caller's slot
    or stable memory of the caller, never written during the call);
  - a field or payload of a value in stable memory (a `getelementptr`);
  - the contents of a box (`unbox`) and a list element (`list.index`)
    reached from a borrowed parameter or a constant through fields,
    payloads, boxes, and list elements: a borrowed root keeps its
    containers alive for the whole call and is never updated in place
    (the reasoning of rc.go `projKind`).
- Tag, payload, and field reads of a value in stable memory load from
  it (no aggregate copy, no alloca).
- A call passes a value in stable memory to a parameter by reference
  that is not `var` as its address; nothing is copied into a slot. A
  `var` parameter still gets a copy (the callee writes it back).

## Consequences

- The LLVM build of the self hosted checker is about 5% faster on the
  selfhost input (20 runs: 5,435 to 5,167 ms, interleaved with the
  previous build); its output is unchanged on every repository run.
- Enums by reference with only payload reads from the reference (no
  address passing) measured neutral (5,451 to 5,565 ms against 5,486 to
  5,545): the copies into call slots were the cost.
- Whether a box or list element is stable depends on borrow inference
  (rc.go `inferBorrows`): an owned parameter's containers may be
  released during the call, so their contents are copied as before.
- The arm64 backend is unchanged.

## Alternatives

- **Payload reads through an alloca of the loaded enum** (0145's
  state): every match copied the enum.
- **Pass box and list element addresses whatever the root**: unsafe; an
  owned root may release the box before the callee reads it.
