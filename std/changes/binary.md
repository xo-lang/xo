# std/binary changes

## 2026-10-20: port: std/binary from Go

- Kind: port
- Commit: (filled in after the push)
- Found by: survey
- Go reference: encoding/binary/binary.go and varint.go (go1.26.1)
- Test: std/native/binary_test.xo; internal/llvm/testdata/golden/binary_basic

New module: byte order writes and reads (16, 32, 64 bit, both orders, signed reads) and varints, with Go's results.
