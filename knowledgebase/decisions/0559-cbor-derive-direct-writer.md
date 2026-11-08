# 0559: derive Cbor writes the wire form directly, without a cbor.Value tree

Status: Proposed (Xo Lang 2, 2026-11-07)

## Context

`cbor.marshal(v)` of a type that derives Cbor (decision 0527) builds a
`cbor.Value` tree with the generated `cbor_wr__T` functions, encodes the tree
(`cbor.encode`), and drops it. After 0a3f3ea4d (deterministic key order by
comparing keys: bench/perf/cbor umarshal 0.442 of main, canon 0.353) the Xo
vs Go survey's row 2 (bench/results/xo-vs-go-survey-2026-11-07.md) is at
about 6.2 ms against Go's 4.3 ms on linux/arm64.

A profile of that build (Mac, XO_PROF=cpu at 10 kHz, 2,000 marshals of the
bench document, 2026-11-07):

- `enc_item` 56% cumulative (26% flat: the match over Value variants);
- `drop cbor.Value` 25.6% cumulative and `obj.each` 13.4% (releasing the
  tree's Lists and pairs): about 40% of the run is the tree's life;
- the key order (direct_keys, keys_sorted) about 6%.

The lead's step 1 (emit each struct's fields in canonical order so the
encoder skips its sort) changes nothing on this benchmark: Rec, Nested and
Doc already declare their fields in canonical order, so keys_sorted passes
and no sort runs. The tree is the gap.

fxamacker/cbor (Go's twin) encodes a struct with a cached per type field
list straight into one buffer: no intermediate tree.

## Decision

`derive Cbor` also generates `cbor_put__T(v: T, det: Bool, var out:
BytesBuilder)` for a struct (and an enum), which writes T's encoding into out
directly, and the generated `marshal` calls it instead of
`cbor.encode(cbor_wr__T(v))`:

- Each field's key is a constant: its encoded head and text (or integer for
  `keyasint`) are written with one `out.write` of a constant Bytes made at
  derive time.
- Field order: declaration order when `det` is false (today's preferred
  bytes, unchanged), and the canonical order (RFC 8949 4.2.1: encoded key
  length, then bytes) when `det` is true, computed at derive time; the
  generated function holds both orders as straight line code, so no runtime
  sort and no package-level cache.
- `omitempty` and `omitzero` fields: the map head needs the count, so the
  writer counts the present fields first (a few compares), then writes.
- `toarray`: an array head and the fields in declaration order.
- Field values: Str, Int, Float, Bool, Bytes and Option through new std/cbor
  helpers (`put_text`, `put_int`, `put_float`, `put_bytes`, `put_head`:
  the encoder's existing functions made public for generated code, as
  `to_str` and the readers are); a List through a head and a loop; a
  Map[Str, V] through `sorted_keys` when det; a nested derived type through
  its own `cbor_put__`; a `cbor.Value` field (and Time, Duration) through
  enc_item.
- `cbor_wr__T` stays (other generated code and `to_value` use it).

Output is byte identical to today's for both modes; the encoder's own
deterministic sort stays for Value trees and Map fields.

## Consequences

- Expected: the tree's ~40% and most of enc_item's dispatch go; umarshal
  near Go's 4.3 ms. Measure with ab-host bench/perf/cbor umarshal and canon
  (n of 6 or more), and the binary size of a program with a few derived
  types (`xo dev size --by-cause`): a second writer per type is more code.
- Compiler change (compiler/check/derive.xo): the fixed point set's
  selfbuild, codegen and wasm checks; std/cbor gains public helpers (an API
  addition, listed in spec/std.md as the generated code's helpers).
- Tests: the cbor corpora (typed cases against fxamacker) on go, arm64 and
  llvm with both modes, the goldens cbor_case and cbor_enum, and a golden
  with omitempty, keyasint, toarray, a Map field and a nested type.

## Alternatives

1. Keep the tree and make it cheaper (pooled or arena Values): the drop and
   the dispatch remain, and a package-level pool is ruled out.
2. Step 1 only (canonical order at derive time): no gain here (above).
3. An SSA-level encoder as `derive Json`'s (compiler/ssa/json.xo): faster
   still, but much more compiler code for a module that is not on a hot
   server path; the generated Xo source keeps the codec readable.
