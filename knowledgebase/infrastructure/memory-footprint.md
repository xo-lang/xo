# Memory footprint of Strs and Lists (native and LLVM)

Last verified: 2026-10-24 (arm64, xo at 0a49d0560, Go 1.26).

Where Xo spends more memory than Go on strings and lists, measured, with the
decisions that follow. Numbers are max resident set size
(`/usr/bin/time -l`) unless noted; the data and probes are in
[bench/results/sortstr-2026-10-24.md](../../bench/results/sortstr-2026-10-24.md).

## A Str costs 56 bytes (Go: 36)

- 2,000,000 `"key${n}"` Strs (4 to 10 bytes) in a `List[Str]`: Xo 114.7 MB,
  Go 77.0 MB (1.49). Slope 1M to 2M: Xo 56.0 B a Str, Go 35.9.
- The 56 bytes: the 16 byte Str value in the list (pointer, `len | off << 32`),
  plus a heap object of 16 header bytes (reference count, capacity;
  `S_HDR` in std/native/llrt/obj.xo) and the text, in a 32 byte size class
  (class 2) with the allocator's 8 byte block header: 40 bytes. Seen with
  `XO_HEAPSTATS=1` after dropping 100,000 Strs: class 2, size 32, 100000
  free blocks, 4,000,000 bytes.
- A long text is cheap (2,000,000 bytes: 5.87 MB against 2.69 MB for an empty
  program); the cost is per object, so many short Strs (map keys, words,
  ids) are what to watch.
- Proposed fix: [0370](../decisions/0370-small-str-inline.md), a Str of up to
  15 bytes stored in its own value (owner's review; nothing built).

## Sorting Strs adds 5n words (Go adds 0)

- `list_sort_str` (std/native/prelude.xo) is a stable index merge sort: its
  scratch peaks at 5n words (merge: the list 2n + pre n + a n + b n; gather:
  the list 2n + a n + out 2n). At 2M Strs the sort adds 80.0 MB (build 114.7
  to 194.7 MB, Go 77.0 MB unchanged: `slices.Sort` is an unstable in-place
  pdqsort).
- A half-size merge scratch does not lower the peak (the gather peak is also
  5n); in-place gather would reach 4.5n at best. Not built.

## A sized integer List takes 8 bytes an element

- `List[UInt32]`, `List[UInt8]`, `List[Rune]`, `List[Float32]` are 8 bytes an
  element on arm64, amd64 and LLVM (10M elements: 82.7 MB, the same as
  `List[Int]`); the Go backend packs (51.8 MB). Using `List[UInt32]` for the
  sort's index lists changed nothing (`--ab`: RSS 1.001, wall noise).
- Use `Bytes` for bytes. Proposed fix:
  [0369](../decisions/0369-packed-sized-int-lists.md). Gotcha:
  [sized-int-lists-take-eight-bytes.md](../gotchas/sized-int-lists-take-eight-bytes.md).

## Measuring this

- `XO_HEAPSTATS=1 ./prog`: blocks on each class's free list at exit, chunks
  carved, MB on free lists (counts the lists since dc49a61ac; before it the
  figure read stale per-class counters and printed 0 MB or a wrapped
  17592186044415 MB).
- A probe that frees its objects before exit shows the class of a given size;
  `/usr/bin/time -l` max RSS A/B against a Go twin settles the rest.
