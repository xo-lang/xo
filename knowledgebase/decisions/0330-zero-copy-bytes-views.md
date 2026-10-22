# 0330: zero-copy Str and Bytes slices (views that share the buffer)

Status: Accepted (owner, 2026-10-21; proposed by Xo Lang Lib 1). Owner: Xo Lang Lib 1, with Xo Lang 2 reviewing the runtime and emitter changes; staged as in Decision item 5.

## Context

Spec core 3.6 says "slices of immutable values are O(1) and share
storage". The native runtime copies: `s[a..b]` and `b[a..b]` allocate and
copy the range (`prelude.llrt_str_slice_at`, `llrt_bytes_slice_at`). Go's
slices are free, so ports of Go code that slice in hot paths pay for it.
Evidence from 2026-10-21, native arm64, instructions retired:

- std/time layouts: every field name and argument sliced out of the layout
  cost enough that recognizing fields in bytes (no slices) cut format by 16%
  (bench/results/regexp-time-csv-vs-go-2026-10-21.md, step 6); Go's
  nextStdChunk compares bytes by index.
- std/mime `parse_media_type` and std/multipart: slicing for each token;
  `raw.str_at` + `llrt_str_slice_at` + `slice_check` are 20 to 30% of the
  profile.
- std/websocket `utf8_cut(chunk)` and streaming parsers cut a buffer at a
  rune boundary and carry the tail: each cut copies the rest.
- std/netip and std/regexp: `rune_at` and substring tests.

Hot paths work around it by indexing bytes, which is why the ports read
like C. A view would let them read like Go.

## Decision (proposed)

Make slicing O(1) for `Str` and `Bytes` as the spec already says.

1. **Representation.** A `Str` or `Bytes` value becomes (buffer reference,
   offset, length). The buffer is the existing reference counted object; a
   view holds a count on it. Whole-value operations (len, index, compare,
   hash, concatenation, printing, system calls) take offset and length.
2. **Immutability makes sharing safe.** `Str` and `Bytes` are values that are
   never modified in place by others; `BytesBuilder` and `StrBuilder` own
   their buffers uniquely until `freeze`, which hands the buffer over. A view
   of a frozen buffer never sees a write.
3. **Memory.** A small view of a large buffer keeps the buffer alive. Rule:
   a view that is stored beyond the function (a struct field, a Map or
   List element, a return from a parse) is compacted (copied) when it is
   under 1/4 of its buffer and the buffer is over 4 KB, at the point it is
   stored; a view used in place is never copied. `s.clone()` copies
   explicitly.
4. **Runtime and ABI.** The layout of Str and Bytes is fixed in the C-free
   runtime (llrt/obj.xo, the static word table, the wasm runtime, and every
   emitter's inline string equality and hashing). Moving to (buffer, off,
   len) changes: string literals in rodata (a view with no count), the
   interned table, `rt.cast[Bytes, Str]`, `raw.str_at`, system call buffers
   (raw.bytes_addr = buffer + offset), the Go backend (Go strings and byte
   slices already are views, so it needs no change).
5. **Staged.** (a) Bytes first, with the websocket and mime/multipart ports
   as the measurement; (b) Str; (c) the rule in item 3.

## Consequences

- The slice-heavy ports (time, mime, multipart, websocket, url, http
  header parsing) lose their per-token allocations: expected 15 to 40% of
  their instruction counts, and the byte-index workarounds can go.
- A rune or byte boundary check stays on Str slices (a slice inside a UTF-8
  sequence faults, as now).
- Every place that reads `obj_addr(s) + layout(0)` as the data pointer
  (raw.xo, the emitters' string loads, TLS and socket buffers) must add the
  offset: a wide mechanical change with a fault surface on every backend,
  hence staging and the identity check (scripts/identity) at each stage.
- A one word wider Str and Bytes value (24 bytes: pointer, offset, length,
  or 16 with the offset packed into the length word for views under 4 GB):
  structs holding strings grow; benchmark before choosing.

## Alternatives

- **Keep copying and rewrite hot ports with byte indexing** (what was done
  today). No runtime change; code stays C-like and every new port repeats
  the workaround.
- **Views only for `Bytes`** (stage a only). Smaller change, solves
  streaming parsers; `Str` slicing stays O(n).
- **A separate `BytesView` type.** No ABI change, but a second type in every
  API and conversions at each boundary.
