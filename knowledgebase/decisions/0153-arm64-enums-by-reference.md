# 0153: arm64 backend passes enum parameters of 256 bytes or more by reference

Status: Accepted (2026-10-11). Implemented in `internal/arm64/byref.go`
(`bigByRef`, `memOf`, `addrAt`; `byRefOf` takes enums) and
`internal/arm64/isel.go` (`OpTag` and `OpPayload` read from memory, a
large field or payload of read only memory kept there). Test:
`internal/arm64/testdata/run/byref_enum` (TestRun: the Go backend's
output), and every arm64 build of the self hosted compiler in
internal/selfhost. Ports decision 0147's rule (LLVM) to arm64, after
decision 0151 (structs).

## Context

Decision 0151 passes struct parameters of 256 bytes or more of direct
calls by address on arm64; enums of that size still went as their
words, through registers and stack slots, in both directions for `var`
ones. The self hosted parser's and checker's `Expr`, `Stmt`, and
`Pattern` are such enums, matched and passed down at every node, and the
LLVM backend has passed them by reference since decision 0147.

## Decision

- An enum parameter of a direct call of 256 bytes or more is passed by
  reference like a struct (decision 0151's convention: one integer word,
  the address of the value's words in layout order; `var` ones are
  updated in that memory with its dataflow, the first of each type is
  the home of the type; functions called through function values, main,
  and Go mode keep the plain convention).
- `tag` and `payload` of an enum in memory (a read only parameter, or a
  value its home holds at every use) load the tag word or the payload's
  words from it, not the whole value.
- A field or payload of 256 bytes or more of read only memory is not
  loaded: it is the address of its words in that memory (nothing writes
  it during the call), read in place where used and passed on by address
  to a parameter by reference (decision 0147's stable memory, for fields
  and payloads; box contents and list elements are not taken).

## Consequences

- The output of the arm64 build of the self hosted SSA builder is byte
  identical before and after (selfhost/check with every stage, 3.4
  million lines); the binary went from 29 MB to 28 MB. Speed: see
  bench/results/selfhost-ssa-2026-10-10.md (arm64: enums by reference).
- A large field of a read only struct parameter (the checker inside the
  builder) is now passed on in place too, where it was loaded and
  stored into frame memory.

## Alternatives

- **Box contents and list elements under a borrowed root** (the rest of
  decision 0147): needs the borrow facts of InsertRC in the backend; not
  done here.
- **Load the enum whole at entry**: keeps every word live across calls,
  the cost decision 0151 removed for structs.
