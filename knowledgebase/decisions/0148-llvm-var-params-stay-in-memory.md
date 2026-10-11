# 0148: LLVM backend keeps `var` parameters passed by reference in memory

Status: Accepted (2026-10-10). Implemented in `internal/llvm/held.go`
(the dataflow: `computeHeld`, `heldIn`, `findMemOnly`, `slotMems`,
`stable`) and `internal/llvm/fromssa.go` (`sfn.held`, `sfn.lazy`; field
reads, `setfield`, call arguments and `var` results, returns, block
parameters). Test: golden program
`internal/llvm/testdata/golden/byref_held` (TestGolden, TestGoldenShared:
the Go backend's output, no leak, faults in callees under task.isolate),
and byref_struct, byref_enum. Evidence:
`bench/results/selfhost-checker-2026-10-10.md` (LLVM experiments).
Extends decision 0145; this is the alternative 0145 named "keep the
value in memory for its whole life".

## Context

Decision 0145 passes a `var` struct parameter of 256 bytes or more as a
pointer to the caller's memory, but the callee still loaded the whole
value at entry, kept it as an SSA aggregate, stored it into that memory
before each call that took it, loaded it again after, and stored it at
every return. A field read was an extractvalue of the entry (or last
reloaded) aggregate, so every field of the self hosted checker's
`Checker` (53 fields, about 1 KB) stayed live across every call: the
landing pads release every field of the value they hold. `set_type`, a
ten line function, compiled to 12,474 instructions.

## Decision

The lowering computes, for each `var` parameter passed by reference that
is the home of its type (decision 0145: the slot of every call that
passes a value of that type by reference), which SSA value its memory
holds before each instruction: a forward dataflow over the blocks
(reverse postorder to a fixpoint) where

- a call stores its argument into the home slot (the memory then holds
  that argument), and a `var` parameter's callee writes back into it (the
  memory then holds that result of the call); a landing pad of such a
  call holds nothing known (the callee's unwind exit writes zero
  values);
- a `setfield` of the held value is done in place (the memory then holds
  the result);
- a block parameter that every incoming edge passes while the memory
  holds it is held at the block's start (no phi is emitted);
- otherwise edges must agree, or the state is unknown.

With it:

- a field read of a held value loads that field from memory;
- `setfield` of a held value stores the field in place;
- a call argument or a return value already in its memory is not stored
  again;
- a value held at every one of its uses (the parameter itself, values
  updated in place, `var` results of calls, block parameters passed in
  place) is never loaded whole; a use that needs the whole value (a by
  value argument, an aggregate return elsewhere) loads it there;
- all operands of a call, and all write backs of a return, are read
  before anything is stored (two `var` parameters of one type share the
  home slot within a call).

## Consequences

- The LLVM build of the self hosted checker is about 40% faster on the
  selfhost input (20 runs: 5,139 to 3,090 ms, interleaved with the build
  of decision 0147), close to the Go checker (2,950 ms); output
  unchanged on every repository run, 1,500 checker mutations, and 400
  module mutations.
- Only the home memory of each type is tracked: a second `var`
  parameter of the same type keeps the 0145 behavior.
- The arm64 backend is unchanged.

## Alternatives

- **Field by field loads and stores at entry and exit** (measured for
  0145, no faster): the landing pads still used every field loaded at
  entry.
- **An LLVM pass (memory to register promotion) to do it**: LLVM cannot
  know that the landing pads' fields are the ones in memory after a
  call that takes the pointer read only, and does not rematerialize
  loads under register pressure.
