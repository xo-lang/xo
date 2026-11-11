# std/binary changes

## 2026-11-10: perf: written with prelude operations, merged with its implementation

- Kind: perf
- Commit: this push
- Found by: decision 0597 part 2 (fast paths rewritten with prelude operations, --ab each)
- Go reference: encoding/binary ByteOrder and varint.go (go1.26.1), unchanged
- Test: the binary corpus on go, arm64 and llvm; `xo test --std-module=binary` on native and llvm; golden binary_basic on arm64 and Go parity
- Timing: bench/perf/binary fixed on linux/arm64 host 1, --ab against main ed13e688d (7 rounds): wall 0.823 (0.772 to 0.838, n=7)
- Shipped in: not yet released

The fixed size puts write with `BytesBuilder.write_le` and `write_be` (one allocation of the right size)
and the gets combine bytes after one range check, where they stored and loaded the word through raw
memory intrinsics; faster on the bench, and every backend now runs std/binary.xo's bodies (std/native/binary.xo
and std/rt/binary.go are gone).

## 2026-10-21: perf: fixed size words stored and loaded in place

- Kind: perf
- Commit: f0088114
- Found by: Xo Lang 2's allocator report (a per byte builder paid a uniqueness, capacity, and bounds check for each of the 4 bytes)
- Go reference: encoding/binary ByteOrder (go1.26.1)
- Test: internal/stdtest/testdata/corpus/binary (arm64, LLVM); golden binary_basic

`u16`, `u32`, and `u64` in both byte orders make one Bytes of the right size
and store the word in place (a big endian value is byte swapped first); the
getters load the word after one range check. 5,000,000 loops of
`get_u32_be(u32_be(i))`, median of 5: 113 ms to 101 ms (22.6 ns to 20.2 ns a
loop; the allocation is most of it, Go's AppendUint32 66 ms). The Go backend
still uses Go's encoding/binary: these are the runtime's memory intrinsics.

## 2026-10-20: port: std/binary from Go

- Kind: port
- Commit: 4c5b0df2
- Found by: survey
- Go reference: encoding/binary/binary.go and varint.go (go1.26.1)
- Test: std/native/binary_test.xo; internal/llvm/testdata/golden/binary_basic

New module: byte order writes and reads (16, 32, 64 bit, both orders, signed reads) and varints, with Go's results.
