# 0198: The xo1 speed work: an empty list allocates nothing, xo1 cached on its own inputs

Status: Accepted (2026-10-13). Implemented on the xo1-speed branch.
Evidence: `bench/results/xo1-speed-2026-10-13.md`.

## Context

Agents waited for two things (owner, 2026-10-13): the build of the self
hosted driver xo1 that every native `xo build|run|test` of a development
`xo` runs (5 to 12 minutes, decision 0178 switch point A), and the
selfhost full tier, whose critical path is the arm64 emitter on
selfhost/xo (about 930 s per emitter, selfhost-parallel-2026-10-12.md).
The profile of xo1 compiling selfhost/xo showed most of its time in
allocation (5.6 billion allocations, each pop from a size class's free
list a cache miss) and in whole bit set passes, and 23.9 GB of peak
memory. Two of the fixes are design choices rather than source fixes:

1. An empty list literal (`[]`, `list.new` without a size) called
   `xo_rl_new(4, size)`: one allocation per empty list, and the MIR's
   `Inst` alone has five list fields that default to `[]`.
2. internal/selfdriver keyed the built xo1 on a hash of every embedded
   file, so a change anywhere in std (a runtime port step, a Windows
   layer) rebuilt xo1 although the compiler had not changed.

## Decision

1. **`[]` is the shared immortal empty list.** Both arm64 emitters and
   both LLVM emitters lower `list.new` without a size to a call of
   `xo_rl_empty` (the runtime's static empty list, count
   `XO_RC_IMMORTAL`), and `xo_rl_grow` turns that list into a fresh one
   of capacity 4 on the first push. Nothing else changes: a shared list
   is already copied before an update (`reuse`), releasing an immortal
   count does nothing, and the rc pass's "a new list is unique" only
   lets a push skip the copy, which lands in `xo_rl_grow` because the
   empty list's capacity is 0. A real-time function (decision 0133) can
   now build a value with a defaulted `[]` field without XO-F007.
2. **xo1 is cached on its own inputs.** The source tree stays keyed on
   every embedded file (xo1 reads std and third_party from it), but the
   binary moves to `<user cache>/xo/xo1/bin/<binkey>/xo1`, where binkey
   hashes the `xo` version, the host, every file under selfhost/, and
   the Xo files of the std modules selfhost reaches through `use` lines
   (followed through std, with each module's native implementation and
   the C path of the llrt modules): `selfdriver.BinaryInputs`. The C
   runtime (std/llrt), third_party, and the other std modules are not
   inputs: the programs xo1 builds read them from the new source tree.

## Consequences

- xo1 compiling selfhost/xo: see the results file for the before and
  after figures (time, allocations, peak memory); the assembly is byte
  identical to before for the source only changes and differs by
  intent (the `[]` lowering) for decision 1, where xo1's output stays
  byte identical to the Go emitter's (the fixed point).
- The selfhost goldens of the arm64 and LLVM comparisons were recorded
  again (`RUN='^TestSelfhost(Arm64|Arm64Mutations|LLVM|LLVMMutations)$'
  scripts/selfhost-record-goldens.sh --merge`), sieve's assembly
  goldens updated, and the frozen copy of the LLVM runtime declarations
  (internal/selfhost/frozen_test.go) gained `@xo_rl_empty`.
- A runtime fix that matters to xo1 itself (a crash in xo1) needs a
  change under selfhost/ or a new `xo` version to reach the cached
  binary, or `XO1=<path>` with a fresh build; the source tree beside it
  is always current.
- `XO_HEAPSTATS=1` (the heap by size class at exit) and
  `XO_HEAP_MID_MAX` (the largest block the size classes serve) are
  diagnostics added on the way; the default heap is unchanged.

## Alternatives

- An empty literal as a null pointer: every list operation would need
  a null check; the immortal list needs none.
- Keying xo1 on the whole std but not third_party: a runtime port step
  (decision 0193) changes std/llrt and std/native/llrt every few hours.
- Keying xo1 on the compiled program's import graph computed by the
  checker: exact, but loads and checks selfhost/xo (seconds) on every
  native command; the `use` scan reads files only.
