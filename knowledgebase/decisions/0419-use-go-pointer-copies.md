# 0419: Go pointers to basic types in `use go` are copies (`T?`)

Status: Accepted (owner, 2026-10-29; proposed by Lang 1 at the lead's request)

## Context

`use go` skips every signature with a pointer to a non handle type (`*bool`, `*string`, `*int`,
`*float64`, `*int32`). The 2026-10-29 survey counts about 90 such skips in the 87 packages, mostly
fields of options structs (excelize's `CalcPropsOptions.CalcCompleted *bool`, go-redis's
`Document.Payload *string`, `JSONArrIndexArgs.Stop *int`), where Go uses a pointer to mean "optional".
Xo has no pointers; its optional is `T?`.

## Decision

1. A pointer to a basic type (`*T`, T a bool, string, integer or float type) in a field, parameter or
   result maps to `T?`: Go's nil is None, a non nil pointer is Some of the value it points to, read at
   the boundary. From Xo, None is nil and Some(v) is a pointer to a fresh copy of v.
2. **The value is a copy, not an alias.** A Go function that keeps the pointer and writes through it
   after the call (or a field Go changes later) is not seen by Xo; Xo reads the field again to see a
   new value. Spec 4.4 states this rule.
3. Functions whose purpose is writing through the pointer stay skipped: sync/atomic's `*int32` and
   `*int64` operations, a `flag.*Var(p *T, ...)` that stores the pointer, and any parameter the binder's
   writes-through check (below) flags. The binder notes a parameter Go assigns through (`*p = ...` in
   the function body, when the source is available to go/types; otherwise a name heuristic: `*Var`
   functions, `dst`, `out`) with a skip line naming the copy rule.
4. Pointers to structs stay handles (decision 0041); pointers to slices, maps and pointers stay skipped.

## Consequences

- Options structs with optional fields bind (survey numbers in the push).
- A Go library that updates an optional field in place (rare for options) is read again by its getter.

## Alternatives

- **A `go.Ref[T]` handle** (a Go `*T` with get/set): keeps aliasing, but every optional field becomes a
  handle the program must allocate; reserved for later if a write-through API matters.
- **Keep skipping**: the options structs stay unusable from Xo.

## As built (Lang 1, 2026-10-29)

- gobind mapType: a pointer whose element is a basic type other than
  unsafe.Pointer maps through basicPtrConv to `Option[T]`; toXo checks
  nil and copies `*p`, toGo allocates a fresh copy and passes its
  pointer. Binding format 21.
- The writes-through check is the name heuristic only (no body
  analysis), and it applies only to pointers to basic types (a pointer
  to a struct is a handle and aliases anyway): a parameter is skipped
  when its package is sync/atomic, its function's name ends in `Var` or
  starts with `Scan`, or its name is one of dst, out, ptr, addr, p,
  result, dest. The skip line says "Go writes through ..., which Xo
  passes as a copy (decision 0419)", and the survey lists every such
  skip so a false one (a read-only `p *int`) is visible.
- Survey (87 packages, 2026-10-29): of 70 writes-through skips with v,
  val and value also in the name list, 53 were real (39 sync/atomic, 14
  flag `*Var`) and 17 false (zap's `Boolp(key string, val *bool)` and
  its kin read `*val` at once), so v, val and value were dropped.
- Tests: gobind TestBasicPointers, cmd/xo TestUseGoBasicPointers.
