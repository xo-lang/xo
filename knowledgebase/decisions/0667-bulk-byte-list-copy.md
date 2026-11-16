# 0667: BytesBuilder.write_list and Bytes.from_list, a List[UInt8] copied in bulk

Status: Accepted (owner, 2026-11-16; the speedup claim still needs its host 1 --ab before landing; proposed by Xo Lang 3, 2026-11-15; the lead asked for the primitive
for std/brotli's ring buffer writes, after the decoder of decision 0666)

Landed: d18e1c059 (the prelude functions, golden bytesbuilder_write_list,
internal/arm64 TestWriteListBulk) and std/brotli's ring writes, both on
2026-11-15, before the owner's acceptance; task #142 (2026-11-16) found
nothing left to land. The host 1 `--ab` row (ab-host.sh, --release,
against the decoder before): wall text 0.637 (0.634 to 0.67), html 0.391
(0.385 to 0.403), mixed 0.656 (0.648 to 0.671), max RSS 1.0, in
std/changes/brotli.md.

## Context

A `List[UInt8]` reached a `BytesBuilder` or `Bytes` only through one
`write_byte` per element. std/brotli keeps its ring buffer as a
`List[UInt8]` (it is written in place by the copy commands, which Bytes
cannot be), and write_ring_buffer, a `write_byte` per byte, was 23% of a
text decode on the Mac (bench/perf/brotli twin, 2026-11-15). std/flate
has the same need and solved it inside std/native/llrt (flate.list_bytes,
list_range_bytes), which a std module outside the native runtime cannot
call. Measured, 131 MB appended on the Mac (`/usr/bin/time -l`):

| backend | runs | write_byte loop | write_list |
|---|---|---|---|
| arm64 | 64 KiB | 1.99G instructions | 77M |
| arm64 | 100 bytes | 2.12G | 555M |
| LLVM | 64 KiB | 10.1G | 80M |
| LLVM | 100 bytes | 10.3G | 1.8G |

## Decision

Two prelude functions:

- `fn (var b: BytesBuilder) write_list(l: List[UInt8], from: Int, to: Int)`
  appends `l[from..to]` (Go's `b.Write(l[from:to])` on a `[]byte`); a
  fault at the caller's line when `from..to` is not a range of `l`
  (`slice F..T out of range for List of length N (BytesBuilder.write_list)`).
- `fn (static: Bytes) from_list(l: List[UInt8]) -> Bytes`: all of `l`,
  copied once.

Neither aliases `l`: a later write to the list leaves the result as it
was.

They are native prelude functions (std/native/prelude.xo
`bytesbuilder_write_list`, `bytes_from_list`, found by the SSA builder's
impl_of as Bytes.zeroed is), not new `rt.*` intrinsics, so no backend's
emitter changes:

- native (arm64, amd64, LLVM): a `List[UInt8]` is packed (decision 0369),
  so its bytes are read by address at `rt.layout(2)`: a run of 256 bytes
  or more is appended as one copied Bytes, a shorter one eight bytes a
  `write_le` (one `rt.rs_push_word`); `from_list` is one `rt.mem_copy`
  into a new Bytes.
- wasm keeps a word an element (gotchas/packed-list-raw-copy-wasm.md):
  eight elements are packed into each `write_le`'s word (a wrapping shift:
  a checked one faulted on a high byte of 128 or more).
- the Go backend: `append(b.b, l.s[from:to]...)` and `Bytes(l.s)` (a
  conversion to Bytes, a Go string type, copies).
- the fault goes through `rt.fault_at` with an `rt.Loc` parameter, so it
  names the caller's line on every target, as the Go backend's FaultHere
  does (fault.raise_str_at reaches the native heap, which a WASI module
  cannot call).

## Consequences

- std/brotli's write_ring_buffer is one call; the Mac survey's xo/go
  ratios went from text 1.82, html 2.34, mixed 1.29 to 1.17, 1.12 and
  0.99 (survey only; the claim is the host 1 `--ab` row).
- A change inside xo's build closure (std/prelude.xo, std/native): gated
  on selfbuild and seedbuild. Tests: the golden
  bytesbuilder_write_list (arm64, LLVM, Go parity, wasm) and
  internal/arm64 TestWriteListBulk, an instruction ceiling (400M and
  1.1G against the byte loop's 2G) that fails if write_list falls back to
  a push per byte.
- flate.list_bytes and list_range_bytes could call these; left as they
  are (native runtime code, already one copy).

## Alternatives

- New `rt.rs_write_list` intrinsics lowered by every emitter and the wasm
  runtime: more code in four places for the same copy; the native
  prelude reaches the list's bytes with the intrinsics that exist.
- A `List[UInt8]` slice as a view, then `write(Bytes)`: lists have no
  views, and a view type would be a language change.
- Keeping brotli's ring as Bytes: Bytes is immutable (core 3.6), and the
  copy commands write the ring in place.
