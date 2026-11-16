# 0671: List[T].filled(n, x), a list of n copies of x made in bulk

Status: Accepted (lead for the owner, 2026-11-16 overnight; proposed by
Xo Lang 3, 2026-11-15, for std/brotli's encoder, after decision 0667)

## Context

A list of n zeros, or of any one value, was built with a push per
element: `List.with_capacity(n)` and n pushes. std/brotli's encoder
spends 64% of a quality 6 encode doing that, for its 4 MB ring buffer
(54%) and its h5 hash tables (10%) (bench/perf/brotli enc6, Mac
XO_PROF=cpu, 2026-11-15), where Go's make([]T, n) allocates cleared
memory in bulk. No existing form does it in bulk: `List.concat`
doubling is slower than the pushes (5 ms against 1.8 ms for 4.2 MB),
and only Str and Bytes have `repeat`, and `Bytes.zeroed` has no
List[UInt8] form.

## Decision

1. A prelude function `List[T].filled(n: Int, x: T) -> List[T]`: n copies
   of x, Go's `make([]T, n)` and a fill (`slices.Repeat([]T{x}, n)`). A
   fault for n < 0, worded as `Bytes.zeroed`'s
   (`negative length N for List.filled`).
2. Native targets (std/native/prelude.xo list_filled): from 64 elements a
   List[UInt8] (packed, decision 0369) is allocated with
   `with_capacity(n)` and set with one `rt.mem_set`, and a List[Int] is
   cleared with one mem_set for 0 or stored a word at a time otherwise;
   the length is then stored into the new object's header, as
   ints_concat does (`rt.layout(3)`). Other element types, fewer than 64
   elements, and wasm (a word an element:
   gotchas/packed-list-raw-copy-wasm.md) push. No new rt intrinsic: the
   prelude uses mem_set, mem_store and obj_addr, which every native
   target and the wasm runtime have.
3. Go backend (std/rt list.go List_filled): make and a fill loop.
4. Verified by behavior: golden list_filled (arm64, llvm, Go parity,
   wasm); a cost guard internal/arm64 TestListFilledBulk (128 MiB:
   UInt8 101M instructions with filled against 1.44G pushed, Int zeros
   98M against 258M; ceilings 300M and 180M, user CPU time off darwin).

## Consequences

- std/brotli's zero_bytes and zero_ints use it: enc6 on the Mac twin
  survey 124.6 to 71.8 ms (Xo / Go 3.60 to 1.90); host 1 --ab in
  std/changes/brotli.md.
- Other std ports that build a zeroed table with pushes (Go's
  make([]T, n)) can call it; it is the form agents port make to.
- The length store relies on a fresh `with_capacity(n)` list held by
  nothing else (Xo Lang 1 reviewed the llvm backend's list.new header
  facts against it, 2026-11-15: rt.mem_store is an untagged store through
  the captured object, so LLVM cannot forward list.new's length 0 past
  it; this holds while mem_store and obj_addr carry no TBAA or noalias
  metadata, which raw_list_set_len's comment names).

## Alternatives

- New `list.filled` SSA op lowered by every emitter: the same bulk fill
  with code in four backends and the wasm runtime; the prelude reaches
  the memory with the primitives that exist.
- `Bytes.zeroed(n).to_list()`: Bytes has no List view or bulk
  conversion, and it would cover only UInt8.
- Keeping push loops in std and reusing buffers across calls: std has
  no package level caches (gotchas/std-port-costly-go-idioms.md), and
  one encode still clears its ring once.
