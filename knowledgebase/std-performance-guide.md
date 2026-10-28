# Writing fast std code (std performance guide)

What the std-against-Go timing audit (bench/results/std-go-timing-audit-2026-10-26.md)
taught, as rules with the measured case behind each. Read it before
porting a Go package or optimizing a std module; gotchas/std-port-costly-go-idioms.md
keeps the symptoms as first observed. Last verified: 2026-10-28.

## 1. Measure before and after

- Every std module has a Go twin under bench/perf/<module> (twin.go.txt,
  twin.txt) and a timing row in the audit; `go run ./scripts/twincheck DIR`
  first, so both sides compute the same thing.
- A speedup claim is a quiet `xo bench --ab` (HEAVY_CLASS=measure, one A/B
  a job, 15 rounds; 30 when the interval grazes a bar: path parts read
  1.165 at 15 rounds and 1.225 at 30).
- Find the cost with `XO_PROF=cpu` on a copy of the bench whose loops run
  10 to 20x longer (the sampler sees about a quarter of CPU time on short
  runs), then measure a ceiling with a throwaway stub before building: path
  parts with clean stubbed out ran in 0.508 of the time, which said where
  to look; uuid bytes with its allocation removed ran at 0.874 of Go,
  which justified decision 0395.
- Read instructions and cycles together. Allocation and cache misses show
  in cycles, not instructions: uuid's allocation was 0.587 of the
  instructions but 0.322 of the cycles; path parts was 1.14x Go in
  instructions and 1.27x in cycles. A change that cuts instructions can
  leave the wall flat (hdec_init in locals: instructions 0.960, wall 0.990).
- Inside a gate, count instructions with `/usr/bin/time -l`; never run
  `xo bench` there (gotchas/bench-ab-inside-gate-deadlock.md).

## 2. Port Go's fast paths, not just its algorithm

- regexp: the literal-prefix scan and the onepass matcher (7.6x to 2.1x).
- base64/base32: whole quanta and words at a time (decode 2.09x to 1.08x).
- big: Go's divBasic with a reciprocal, then its row as two carry chains
  one after the other (be585e555: about 11 arm64 instructions a word to 8;
  quorem 1.30x to 1.01x Go). Check the emitted loop with `--asm=PATH`:
  interleaving two carry chains in one loop moves a flag through a
  register at every word.
- flate: Go's code tables, the bit buffer in locals.

## 3. Allocation

- A small List made per iteration costs an allocation and a free: path's
  clean kept its element offsets in a List; eight locals instead, 67aa9537e
  (parts 0.927).
- Size a builder once: toml's JSON text from the TOML length (bfae1d3c1,
  with local scans: 0.954); big's decimal digits eight to a word into a Str
  of the final length (55af0adc5, 0.958).
- Trim in place, do not slice: `xs[0..n]` on a List copies; big's norm did
  that whenever a top word was zero (9f8b670ce: text 0.945; a7c4d9bc8 read
  the key instead of unpacking it: mul 0.906).
- Do not build text to append it: template made an integer's text, then
  appended it (37% of render's allocations); `"${s.out}${n}"` appends in
  place (a111fa25d, 0.934).
- A table computed per call (crc_table) is a constant in Go; in Xo make it
  a `const` list, since std has no package-level state.
- Use Go's element widths for big tables: flate's hash tables as
  List[UInt32] (packed natively) halved what each stream zeroes and reads
  (035518a51: archive zip 0.959).
- Small Bytes and Str that a function only reads can live in the frame
  (decision 0395, arm64): keep such a buffer's uses to reads, views,
  comparisons and rc, and it costs no allocation.

## 4. Slices are views, not free

- `s[a..b]` and `b[a..b]` are views (decision 0330), but each pays a range
  check, a UTF-8 boundary check on Str, a count and a drop. Offsets win in
  hot loops (path's is_match by offset: 1.70x to 1.00x Go).
- The Str slice check inlines at call sites in a loop (3aa2a7b38); outside
  loops it is a call.

## 5. Word loops and byte loops

- Word loops win on long inputs (CRC, base64, scan.span over a buffer).
- They lose on short ones: toml's escape scan with scan.span measured 1.009,
  utf8_valid's ASCII fast path 1.004; a 5 to 20 byte string is cheaper a
  byte at a time.
- Read through a local index, not a struct field: toml's ws() re-read
  p.i and p.s per byte (local index: part of bfae1d3c1).

## 6. Per-CPU kernels

- std/native/llrt/cpu.xo functions have a portable body and are lowered to
  instructions by the emitters, behind cpu.hw() bits (decision 0231):
  crc32x (aa13f8cd0: gunzip 0.949, unzip 0.893), AES, PMULL, SHA-256,
  ChaCha. wasm and older xos run the body; amd64's crc32 is Castagnoli only.

## 7. Raw memory

- Keep the object alive while its address is in use (raw.alive,
  bigmod.keep).
- Do not copy a List's elements by address unless they are words: a
  List[UInt8] is packed natively and a word an element on wasm
  (3869fcb91, gotchas/packed-list-raw-copy-wasm.md).

## 8. What did not pay

- StrBuilder for small results (no UTF-8 check, but its growth): path
  clean instructions 1.066.
- Counting pieces first for split: a second walk costs more than the
  geometric growth it saves (strsplit 1.07 to 1.50x).
- A per-value memo in a Map (gotcha cf877cd68).

## 9. A new std module

- A Go twin and a timing row in the audit; a Go-generated corpus; a
  profile before optimizing; a std/changes/<module>.md entry per change in
  decision 0276's format.
