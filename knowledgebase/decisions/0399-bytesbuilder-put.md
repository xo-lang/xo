# 0399: BytesBuilder.put_le and put_be overwrite bytes in place

Status: Accepted (lead request, 2026-10-28; Xo Lang 3). Not yet on the website.

## Context

The three linkers (compiler/macho pelink and out.xo, link.xo, elflink.xo) lay
out a section in a `BytesBuilder`, then apply relocations: each patch slices
the bytes (a copy) and writes them back a byte at a time. Xo Lang 2's oracle
put that at 5 of 10 ms of a Windows link's layout. `BytesBuilder` had
`write`, `write_byte`, `write_str`, `write_rune`, `len` and `freeze`
(spec/std.md): nothing stores below `len`.

## Decision

`fn (var b: BytesBuilder) put_le(at: Int, v: Int, n: Int)` and `put_be`: the
low `n` bytes of `v` (`n` 1, 2, 4 or 8) stored over `b[at..at + n]`, lowest
first (`put_le`) or highest first (`put_be`), in place; a width not in
{1, 2, 4, 8} or a range outside `0..len()` is a fault. The length does not
change. No `put_byte`: `put_le(at, x, 1)`. Like Go's
`binary.LittleEndian.PutUint32` on a slice, `v` is truncated, not range
checked (a linker patch computes a wrapped value).

A builder is move only (core 5.3), so it is unique: the store goes into its
string object. Under `--inline-str` a short builder may be an inline Str, which
has no bytes to store into: it is copied to an object first.

Lowering: compiler/ssa call.xo calls the native prelude's `llrt_rs_put`
(std/native/llrt/raw.xo `put`: `rs_push_word` of no bytes makes the builder
unique, then one `mem_store` of the width), which the inliner can fold into a
call-free store; the Go backend has `M_put_le`/`M_put_be` (std/rt/list.go);
the wasm runtime `$rs_put` (compiler/wasm/rt.xo; a bad width or range traps
there).

## Consequences

- Linkers can patch relocations with one store each (Xo Lang 2 and xo-lang-4
  convert pelink/out.xo, link.xo and elflink.xo).
- Timing: see std/changes/prelude.md.
- A native inline fast path (arm64: count 1, in range, one `str`) is a
  follow-up if the call shows in the linkers' profile.

## Alternatives

- `set_byte` only: a relocation is 4 or 8 bytes; four stores and a loop.
- A mutable `Bytes` slice type: a larger language change, no owner for the
  unique-or-copy rule.
- A `BytesBuilder.patch(at, Bytes)`: allocates the patch bytes first, the cost
  being removed.
