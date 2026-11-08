# 0564: One convention for wide values (by pointer across arm64, x86-64 and LLVM)

Status: Proposed (Xo Lang 1, 2026-11-08; owner decides)

## Context

A wide value is a struct, tuple or enum of 256 bytes or more
(BY_REF_MIN in compiler/arm64 byref.xo and compiler/llvm gen.xo). The
backends pass wide values in several ways, decided one case at a time:

| Where | arm64 and x86-64 (compiler/arm64) | LLVM (compiler/llvm) |
|---|---|---|
| parameter of a direct call | by address, struct and enum (0151, 0153); a `var` one updated in place (0148's dataflow) | by pointer, struct and enum (0145); `var` ones held in memory (0148) |
| tuple parameter | by value (words) | by value (first class aggregate) |
| parameter of an indirect call, a closure, an entry, a function whose address is taken | by value | by value |
| result | by value: result words in the caller's stack argument area | by value: a first class aggregate |
| count helpers (retain, release, drop) | no by address path found in compiler/arm64 (to confirm in step 1) | by pointer, struct, tuple and enum (rc_by_ptr) |
| equality helper | by address since 0a1b1e803 (closures.xo eq_by_ref, tuples included) | by pointer since 0a1b1e803 |
| box, optional payload, closure environment, by reference slot | words stored one at a time | one first class store; since cacc07cd0, field by field or memcpy past 4 KiB (WIDE_STORE_MIN) |
| a wide SSA value inside a function | words in virtual registers, spilled | a first class aggregate in SSA registers |

Each by value case has cost something once a program had a big enough
value:

- Equality (bench/results/wide-values-llvm-2026-11-08.md): one `==` of
  a 4,096 word struct built 801,600 bytes of native __text (423,752
  without it), took 3.77 s of native emit and 188.8 s of clang. Passing
  both values by address (0a1b1e803) made it 424,576 bytes, 0.28 s and
  15.1 s.
- Stores into memory (cacc07cd0): clang expands one first class store
  of a 4,096 word struct into a store per scalar. wbC (internal/arm64
  wide_box) went from 7.9 to 4.0 s of clang -O1 (hello world: 2.5 s),
  the golden wide_store from 18.4 to 6.8 s.
- What cacc07cd0 leaves: wbN's `main` alone still costs 1.5 s of clang
  (4.04 against 2.55 s with its body emptied), in the Machine
  Instruction Scheduler (+0.57 s), EarlyCSE (+0.45 s) and Machine Copy
  Propagation (+0.12 s). They work on non constant wide values kept as
  SSA aggregates (`b.core.s7.s7.s7.f7 = 77` is a chain of whole L3, L2
  and L1 extracts and inserts), which clang splits into thousands of
  virtual registers.
- Code size before 0145, 0151 and 0153: by value releases copied every
  word of a big struct to the stack first (xo built by LLVM: 11.15 MB of
  its 89.8, bench/results/xo-code-size-2026-10-18.md).

Each was a separate fix with its own threshold test (`g.size(t) >=
BY_REF_MIN`, `8 * g.nwords(t) >= BY_REF_MIN`, `WIDE_STORE_MIN`), with
tuples in some and not others. A new helper, or a new place that writes
a value to memory, starts by value again.

## Decision (proposed)

One rule, the same in compiler/arm64 (arm64 and x86-64) and
compiler/llvm: a value of a struct, tuple or enum type of BY_REF_MIN
(256) bytes or more lives in memory and moves by address.

1. **Parameters**: every call passes a wide argument by address,
   tuples, indirect calls, closures and functions whose address is taken
   included; the function type's ABI says so, so a function value and a
   direct call agree. A callee never writes through a non `var` pointer;
   a `var` one keeps 0148's in place updates. Entries (`main`, C
   exports) keep the platform ABI.
2. **Results**: a wide result is written by the callee into memory the
   caller provides (LLVM `sret`; on arm64 and x86-64 an address in a
   register, as AAPCS64's x8 and the SysV hidden pointer), not returned
   as result words.
3. **Helpers**: count, equality, hash, clone, Json and the other derive
   helpers take wide values by pointer (equality already does on every
   backend, count helpers on LLVM).
4. **Values inside a function**: a wide SSA value gets a home (a frame
   slot, or the memory of the parameter or result holding it). Field
   reads load from it, a field set stores into it in place when the old
   value is dead (0148's dataflow, from `var` parameters to every wide
   value), and a copy is a memcpy. In LLVM no first class aggregate of a
   wide type is loaded, stored, inserted into or extracted from.
5. **One predicate**: `wide(t)` in compiler/ssa, used by both backends
   and every helper generator, replacing BY_REF_MIN's three spellings and
   WIDE_STORE_MIN.

The language does not change: wide values stay values (copy semantics,
`==` by value). Only generated code changes.

## Consequences

- LLVM no longer sees wide first class aggregates: the remaining 1.5 s
  of wbN's `main` should go, and so should the cases cacc07cd0 leaves (a
  wide value returned from a call, then stored).
- Code size: by address removes the word copies at every call site, as
  0151 did for direct calls; indirect calls and closures over wide values
  gain the same.
- Indirect calls and function values change ABI (point 1). One xo
  compiles every caller and callee, so nothing is mixed; the `use c`
  boundary and C exports keep the platform ABI, through cabi's shims, as
  today.
- Wide results by `sret` (point 2) change the result words layout
  (emit.xo out_arg_bytes); the fixed point and every backend golden
  must still pass.
- A small wide value (256 bytes to about 1 KiB) can be slower by
  address than in registers in a hot loop. The plan measures that before
  each default changes; BY_REF_MIN moves in one place.
- Debug info: a wide value with a home is described by its location (a
  frame slot), which 0252's DWARF variables handle for spilled values.

## Plan (each step measured before the next)

1. The `wide(t)` predicate at today's sites, no behavior change
   (identity on every golden, arm64 and LLVM).
2. Tuples by address in direct calls and helpers (points 1 and 3 for
   tuples): identity apart from programs with wide tuples, the self
   build, `xo bench --ab` self.
3. Wide results by `sret` (point 2), both backends: the self build's
   time and __text, LLVM clang time of wide_store and wbN.
4. Wide SSA values with homes in LLVM (point 4): clang time of wbN
   (target within 0.3 s of hello world) and of xo's own LLVM build.
5. Indirect calls and closures (point 1), the ABI change, last.

## Alternatives

- **Keep fixing case by case** (today). It works, but each new helper
  or memory write starts by value; the equality case cost a 188.8 s
  clang build before anyone looked.
- **A higher threshold** (4 KiB, as WIDE_STORE_MIN): fewer programs
  change, but copying 256 byte to 4 KiB values at calls (the 11 MB of
  2026-10-18) comes back.
- **Leave LLVM's aggregates and run SROA earlier**: SROA is what splits
  them; the split itself is the cost.
- **Box wide values on the heap** (as recursive types are): an
  allocation per value, and copy semantics then need copy on write; far
  more run time than a frame home.
