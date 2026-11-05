# 0519: wasm passes and reference counts values past a leaf limit by memory

Status: Proposed (Xo Lang 1, 2026-11-05).

## Context

The wasm backend keeps a value as its leaves: a struct of 3,700 Int
fields is 3,700 wasm values. A wasm function takes at most 1,000
parameters (the engines' limit, checked by check_arity in
compiler/wasm gen.xo and func.xo), so two goldens added on 2026-11-05 with
the arm64 large-value fixes (881677d3c) do not build for wasm and are
listed in internal/selfhost/testdata/wasm_unsupported.txt:
rc_large_offsets (`Result[D, Str]`, D of 3,700 fields: its reference
count helpers $dup167/$drop167 take 2,000 leaves) and frame_large_value.
A user function taking or returning such a value fails the same way.

What exists: decision 0179 stage 3 (home.xo) passes a `var` parameter of
HOME_MIN (16) leaves or more by the address of a memory home, for direct
calls only; mem_rc retains or releases a wide struct or tuple (more than
WIDE_RC, 16, leaves) in memory field by field through rc_addr_fn. Enums
are not covered, and rc_fn, the leaf form, never routes a wide type to
the address form.

## Decision (proposed)

A type of more than WIDE_LIMIT leaves (proposed 256; below the 1,000
limit with room for a function's other parameters) is passed by memory
everywhere the leaf form could exceed the limit:

1. Reference counting. rc_fn of such a type is its address form
   (rc_addr_fn, extended to enums: the tag, then the live variant's
   fields by mem_rc). A caller holding the leaves in locals stores them
   to a shadow stack slot, calls the address form, and pops the slot;
   mem_rc calls the address form directly (no mem_push of the leaves).
2. Parameters. A parameter of such a type, `var` or not, of every
   function and every call kind (direct, indirect through a function
   value, closures, interface thunks), is an i32 address. The caller
   passes the home of a value it gives up, or a shadow stack copy; a
   by-value callee reads leaves from it as 0179's callee reads a home
   (it never writes a non-`var` home). Both sides of an indirect call
   agree because the choice depends on the type alone.
3. Results. A function returning such a type takes a hidden first
   parameter, the address the caller reserved, and writes the result
   there; the result of a call is a home in the caller.

Types at or below WIDE_LIMIT keep today's ABI, so no program that builds
today changes its code.

## Consequences

- rc_large_offsets and frame_large_value build and run on wasm; both lines
  leave wasm_unsupported.txt in the push that lands steps 1 to 3.
- A call passing a wide value copies it to the shadow stack once when the
  caller still needs it (today the leaves are copied as values).
- Proving: those two goldens, TestSelfhostWasm targeted runs (rc_*,
  conc_*, map_*, atomic_ops, closures), and a new golden with a wide value
  through a closure and an interface method. The self build is not wasm,
  so no fixed point.

## Alternatives

- Raise or lift check_arity: the engines refuse modules past 1,000
  parameters, so the binary would not load.
- Box every wide value on the heap: the same memory traffic plus an
  allocation and a reference count per value.
- Address form for rc only (step 1): fixes rc_large_offsets's helpers but
  not a user function taking D by value.
