# 0151: arm64 backend passes struct parameters of 256 bytes or more by reference and keeps `var` ones in memory

Status: Accepted (2026-10-11). Implemented in `internal/arm64/byref.go`
(`byRefOf`, `keptResults`, the dataflow `computeHeld`, `heldIn`,
`findMemOnly`, `slotMems`, `lazyResult`; the call `byRefCall`, the
return `byRefReturn`, the entry `byRefEntry`), `internal/arm64/isel.go`
(field reads, `setfield`, block parameters, `words` of values held in
memory or read only parameters), and `internal/arm64/emit.go` (the
frame's memory for by reference arguments, `frameInfo.memOff`, source
kind `sMem`). Tests: `internal/arm64/testdata/run/byref_var` (TestRun:
the Go backend's output, no leak), the LLVM goldens byref_held,
byref_struct (TestGoldenRun, TestGoldenRunShared), and every arm64
build of the self hosted lexer, parser, checker, and SSA builder in
internal/selfhost. Ports decisions 0145 and 0148 (LLVM) to arm64.
Evidence: `bench/results/selfhost-ssa-2026-10-10.md` ("arm64: large
struct parameters by reference").

## Context

The arm64 backend flattens every value into 64 bit words (decision
0049) and passed a struct parameter as its words: the first eight of
each class in registers, the rest in 8 byte stack slots, and a `var`
parameter's new value came back as more result words, which the callee
stored into the caller's stack argument area. The self hosted SSA
builder's `Builder` (about 300 words, it holds the checker) is a `var`
receiver of almost every function: each call copied 2.4 KiB in and 2.4
KiB out through the stack, and the callee spilled every word at entry.
The arm64 build of the builder took 13 times the Go builder's time
(LLVM, which did this in decisions 0145 and 0148, 1.56 times), and its
`Builder.std_call` reached 280,000 instructions (past the 1 MiB reach
of conditional branches, gotchas/arm64-tbz-range.md).

## Decision

- A parameter of a direct call whose type is a struct of 256 bytes or
  more (`byRefMin`, as LLVM) is passed as one integer word, the address
  of memory holding the value (8 bytes per word, in layout order, Float
  words as their bits). Functions called through function values
  (closure bodies, wrappers, task entries, functions used as values),
  main, and Go mode keep the plain convention.
- A `var` such parameter is updated in that memory; its new value is no
  longer among the result words. The first one of each type is the home
  of the type: every call of the function that passes a value of the
  type to a `var` parameter by reference passes the home's address. The
  dataflow of decision 0148 (ported from internal/llvm/held.go) tells
  which SSA value the home holds before each instruction: field reads
  of a value held at every use load the field, `setfield` of the held
  value stores the field in place, calls and returns skip storing the
  value the memory already holds, and such a value is never loaded
  whole (a use that needs it whole, a by value argument or a release,
  loads it there).
- A non `var` such parameter is read only: nothing writes its memory
  while the callee runs, so the callee reads fields from it and passes
  the same address on to calls taking it by reference. A caller passes
  in place a read only parameter of its own, or a value its home holds
  when no `var` argument of the same call is passed in that home
  (`s.merge(s)` copies); otherwise it stores the value into memory of
  its frame.
- Other by reference arguments (a caller with no home of the type, a
  second `var` argument of a type in one call) go through memory of the
  caller's frame (`frameInfo.memOff`, one area per type and position),
  loaded back after the call. A second `var` parameter of one type is
  loaded at entry and stored back at every return. All operands of a
  call, and all write backs of a return, are read before anything is
  stored.

## Consequences

- The arm64 build of the self hosted SSA builder is 5.8 times faster
  (`--bench=10 selfhost/check`: 9,434 to 1,630 ms; the Go builder 710
  ms, so 13.2 to 2.3 times the Go builder) and its binary half the size
  (54 to 23 MB); its output is byte identical on selfhost/check and
  selfhost/ssa.
- Register allocation merged an occupancy list by sorting the whole
  list at every assignment; with the by reference convention the
  builder's functions have more, shorter live ranges and the arm64
  build of selfhost/ssa took twice as long (55 to 100 s). A linear
  merge into the list's own storage (`mergeSorted`, regalloc.go) brings
  it to about 60 s.
- testdata/run/far_out_slot passed its 600 word struct by value to show
  a far `$OUT` slot; the call now goes through a function value, which
  keeps the plain convention.
- Not done: enums by reference (decision 0147's LLVM rule), and reading
  a field of a field of a held value without loading the inner field
  whole (`h.a.a.a += n` loads all of `h.a`).

## Alternatives

- **Pass by reference, load the whole value at entry and store it at
  every return** (decision 0145 alone): keeps every word live across
  every call and spills them all; 0148 measured it 40% slower on LLVM.
- **Only `var` parameters**: by value receivers (`fn (c: Checker)
  show`) then load the whole held value and pass 300 words; making them
  read only references removed every such whole load in selfhost/ssa.
