# 0395: small Bytes and Str that do not escape live on the stack

Status: Accepted (the owner approved the direction, 2026-10-28, through the
lead), implemented 2026-10-28 for arm64 (version 1); amd64 keeps the heap
until its emitter has the slot; LLVM and wasm keep the heap. Proposed and
built by xo-lang-5; pass placement and escape rule reviewed by Xo Lang 1.

## Context

std/uuid's `bytes()` row was 1.966x Go's time (bench/perf/uuid, measured
2026-10-28): 39% of the run was allocating and freeing the 16-byte Bytes
each call returns, where Go's `u[:]` slices the value's own array and its
escape analysis keeps that array on the stack. Every native Bytes and Str
is a reference counted heap object (decision 0046), however small and
briefly it lives.

Ceiling, measured before building: a throwaway copy of the bench that
makes each uuid's Bytes once before the loop (the rest unchanged) ran in
0.465 of main's wall (instructions 0.587, cycles 0.322) and 0.874 of Go's
(0.84 to 0.924). Allocation and free cost far more cycles than
instructions.

How much else it could reach: an uncached self-build (heap profile, 8 MiB
sampling) makes about 1.91 billion allocations; its small sites are mostly
lists and structures, and its one small Str site (num.append_int, 5.1%,
40 bytes) qualifies only where it is inlined and consumed. jsontwin
decode's small Strs are decoded fields stored into the result (they
escape). template render's num.append_int (37.5%) was fixed in std instead
(the integer appended to the output in place). Decision 0370's inline Str
measured wall within noise on the Str, json and http benches. So this is
not a broad win: it is uuid's shape, a fixed-size buffer made by an
inlined runtime function and only read.

## Decision

After the native inliner (compiler/arm64 inline.xo, after view_lens; pass
`stack_small`, which XO_PASSES can turn off), on arm64:

1. A candidate is an `rt.rs_with_cap(c)` whose capacity resolves to a
   constant of at most 64 bytes, through parameters of blocks with one
   incoming edge. Every Str and Bytes object is made by this intrinsic;
   uuid's is made inside internal.uuid_bytes_words and is visible in the
   caller only after inlining, which runs after insert_rc.
2. Escape rule, over the object and its aliases (the results of
   rs_set_len, str_view and rt.cast on it, and the parameters of blocks
   that are not loop headers which its edges pass it to): every use is a
   Retain, Release or Drop, or a call on the read list (Str.len,
   Bytes.len, bytes.index, str.index, the Str and Bytes comparisons,
   rt.bytes_word, rt.obj_addr, rt.mem_load, rt.mem_store, rt.mem_copy,
   rt.bytes_eq16, rt.bytes_high16, rt.utf8_valid). Anything else, a call
   of any other function included, and an edge into a loop header's
   parameter, keeps the heap. The rule reads only the function's own
   post-inline SSA, so cold and warm builds (decision 0283's undecoded
   callees) decide alike; a callee's "borrowed" flag is not used, since a
   callee may retain a borrowed parameter and store it.
3. A candidate becomes `rt.rs_stack(c)`: the emitter reserves 16 bytes of
   header and c bytes rounded to 8 in the frame's memory, stores the
   immortal count (obj.xo IMMORTAL) and the capacity, and makes the Str
   (the slot's address, length 0). Retains and releases stay: on the
   immortal count the emitted retain and release change nothing, so the
   module verifier sees the same reference counting.
4. One slot serves every execution of its site: a value crosses from one
   loop iteration to the next only through a loop header's parameter,
   which the rule rejects, so the object is dead before its site runs
   again. Fiber stacks are fixed mappings that never move.
5. Verifier rule: after the passes that follow (hoist_fields, fold_tuples,
   cse), every rs_stack is checked again; one that escapes fails the build
   with the function and value named.

## Consequences

- uuid bytes (bench/perf/uuid, quiet --ab, 15 rounds): 0.504 of the
  parent's wall (0.479 to 0.525; instructions 0.717, cycles 0.371), 0.965
  of Go's (0.931 to 1.001), from 1.966x.
- Code size grows by the header stores at each site taken.
- Where it fires today, besides uuid: the identity tool over every fourth
  golden against the parent (202 builds, darwin and linux arm64) found 121
  that differ, and the functions whose instructions changed are the ones
  with a small fixed buffer they only read: linux.sigaction (its 32 byte
  old action, which every linux program sets up at start: 101 programs),
  dns.zone_index (10), internal.rt_op (7), proc.collect and windows.pipe
  (2 each); everything else in those binaries moved address only. A
  linux/arm64 Docker run of 12 goldens (proc, tls13_server, websocket,
  strings, uuid and others) passed.
- A push onto a frame object copies it to the heap first (the immortal
  count is not unique), so a buffer built by pushes gains nothing.
- Tests: internal/gen TestStackSmallProperty (generated programs whose
  uuid Bytes are read, viewed, compared, turned back into uuids, faulted
  around, and kept through lists, returns, closures, loop variables and
  calls; native against the Go backend), test/codegen/strs uuid_byte.

## Alternatives

- A no-escape summary per (function, parameter) to accept calls: needs to
  live in BodyFacts so undecoded callees answer it alike (Xo Lang 1);
  version 2, if more sites call for it.
- Inline Bytes as 0370 does Str: two words hold 11 bytes, not 16.
- A per-thread small-object cache in the allocator: speeds every small
  allocation, but keeps the count and free work Go does not pay.
