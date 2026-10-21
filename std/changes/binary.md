# std/binary changes

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
