# 0260: Named wrapping and unsigned operations

Status: Accepted (owner, 2026-10-18), with seed 0.21.0. Step 1 (the
methods, their native and Go bodies, and the SSA builders' inline
lowering: compiler/ssa call.xo word_method; golden int_word_methods)
landed before the seed (34dab50f). Step 3, after seed 0.21.0: std's
7,787 calls (7,557 rt.int_wrap, 230 rt.u64_mul_hi, in 68 files) are
the methods, the generators emit them, scripts/fieldcheck.py reads
them, and internal/repotest TestStdUsesWordMethods rejects the
intrinsics elsewhere in std. The compiler binary and every
internal/llvm golden (arm64 and LLVM backends) build byte identical
before and after.

## Context

The runtime and crypto code (std/native/llrt) writes word arithmetic as
`rt.int_wrap(k, a, b)` with a numeric operation code: 0 add, 1 sub,
2 mul (wrapping like Go's int64), 3 shift left, 4 logical shift right,
5 rotate right, 6 rotate right of the low 32 bits, 7 byte swap. std and
compiler/ hold 7,588 such calls (2026-10-18: code 0 3,521, 5 1,587,
6 1,195, 2 413, 4 368, 3 227, 1 216, 7 61), plus 229 `rt.u64_mul_hi`.
Reading `rt.int_wrap(2, rt.ct_barrier(c), 38)` means remembering that 2
is a multiplication; a wrong code is a silent miscompile of the
program, not a type error (the agents' survey of 2026-10-18 ranked it
first among the frictions of writing runtime Xo).

What exists already:

- `Int.wrapping_add`, `wrapping_sub`, `wrapping_mul` (spec/std.md), as
  prelude functions over `rt.int_wrap` (std/native/prelude.xo).
- `UInt8` to `UInt64` (core 2.1), which fault on overflow like `Int`.
- `<<`, `>>` (arithmetic), `&`, `|`, `^`, `~` on Int.

Why the runtime does not use the methods: a function of a no allocation
module (rc, alloc, sched: decision 0207 design 3) may call only inline
`rt.*` intrinsics, because the SSA builder adds a landing check after
every other call; and a prelude method is a call until the inliner
takes it, which the emitters' chains (cpu.adc pairs, decision 0231's
addendum) do not see through. The crypto code, which has no such rule,
followed the runtime's style.

## Decision (proposed)

1. **Methods, not operators.** Int's word operations are methods,
   discoverable with the others and with one obvious spelling:
   `wrapping_add`, `wrapping_sub`, `wrapping_mul` (as today),
   `wrapping_shl(k)`, `shr_logical(k)`, `rotate_right(k)`,
   `rotate_right32(k)` (the low 32 bits, zero extended),
   `swap_bytes()`, `mul_hi_unsigned(m)` (the high word of the 128 bit
   product as UInt64s), and `lt_unsigned(m)` / `le_unsigned(m)`
   (comparisons as UInt64s). k is taken mod 64 (32 for the 32 bit
   rotation), as `rt.int_wrap` does.
2. **Intrinsics.** The SSA builders lower these methods as they lower
   `rt.int_wrap` today (an inline operation in every emitter, no call,
   no landing check), so the no allocation modules may use them and the
   adcs chains see them.
3. **No new unsigned type and no new operators.** `UInt64` keeps its
   faulting arithmetic; a wrapping unsigned type would double the
   conversions at every boundary of word code, which is Int throughout.
   No `>>>`, `+%` or similar: an operator is not discoverable from the
   type, and Xo keeps one spelling per operation.
4. **Migration.** std and compiler/ move to the methods mechanically
   (a rewrite of `rt.int_wrap(K, a, b)` to `a.METHOD(b)`, checked by the
   fixed point being byte identical, since the lowering is the same);
   `rt.int_wrap` then stays only as the methods' lowering, and a
   repotest check rejects it outside std/native/prelude.xo (and its
declaration in std/native/rt.xo). The crypto
   generators (scripts/gen/x25519fe64, nistecmul4p and keccaklazy,
   Xo ports of the Python scripts with byte identical output) emit the
   methods.
5. **Seed.** An older xo does not know the new methods as intrinsics, so
   the compiler change lands first and std's use of them after a new
   minor version's seeds (gotchas/seed-older-than-intrinsics.md), batched
   with other seed-breaking changes.

## Consequences

- Runtime and crypto code reads as arithmetic: `c.wrapping_mul(38)`,
  `x.rotate_right(13)`, `a.mul_hi_unsigned(b)`.
- User code gets the same operations (hashes, checksums, protocol code)
  without `use go` or a private intrinsic.
- Compile time loops (unrolling a fixed range at compile time) would make
  the Python generators of the field code unnecessary: the unrolled
  chains could be written in Xo directly. They are a separate language
  question and not part of this decision; until then the generators move
  to Xo programs that print the code (owner, 2026-10-18), byte identical
  to today's.

## Alternatives

- **Operators** (`+%`, `*%`, `>>>` as in Zig and Java): shorter in field
  code, but a second spelling of operations that already have names, and
  not found from the type's method list.
- **A wrapping UInt64 (Go's uint64)**: natural for word code, but Xo's
  UInt64 faults on overflow like every integer type; a second unsigned
  type with other rules is a trap, and conversions at every Int boundary
  would cost more text than they save.
- **Keep rt.int_wrap with named constants** (`rt.int_wrap(MUL, a, b)`):
  readable, but still a private intrinsic, still not type checked
  against the operation, and not available to user code.
