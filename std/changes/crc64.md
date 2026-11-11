# std/crc64 changes

## 2026-11-11: refactor: std/crc64 is one file with a native fast path (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 part 2 (rt.bytes_word is no portable intrinsic)
- Go reference: none (no behavior change)
- Test: TestStdCorpora crc64 on go, arm64, llvm
- Shipped in: not yet released

std/crc64.xo holds the API and checksum's body; update is the fast path,
declared bodyless there: slicing-by-8 over tables read a word at a time
(std/native/crc64_native.xo) on the native backends, Go's hash/crc64 on
the Go backend. A portable update would read each table word as eight
bytes, 72 loads for every 8 input bytes, so it stays native.

## 2026-11-04: feature: Poly.Nvme, CRC-64/NVME

- Kind: feature
- Commit: this push
- Found by: the AWS SDK work (S3's default flexible checksum is CRC-64/NVME)
- Go reference: hash/crc64 `MakeTable(0x9a6c9329ac4bc9b5)` (aws-sdk-go-v2's table)
- Test: the crc64 corpus (arm64, LLVM, Go backend): Go's values on 29 buffer lengths and 31 golden strings, every split of update, the table against the bit loop, and the catalogue's check value of "123456789"
- Timing: bench/perf/checksum crc64nvme beside crc64 (same slicing-by-8 as ISO and ECMA)
- Shipped in: not yet released

`crc64.Poly.Nvme` selects the reversed polynomial 0x9a6c9329ac4bc9b5. `checksum(b"123456789", Poly.Nvme)` is 0xae8b14860a799888 as a Go uint64 (negative as an Int, as Go's wider values already are).

## 2026-11-03: port: std/crc64, Go's hash/crc64

- Kind: port
- Commit: this commit
- Found by: the lead's roadmap (priority 2: the hash packages), std-coverage.md planned row
- Go reference: hash/crc64 (crc64.go: slicing-by-8, the ISO and ECMA tables) of go1.26.1
- Test: internal/stdtest/testdata/corpus/crc64 (Go's values: its golden strings and pseudo random buffers of lengths that cross the word step and the block, split at several points, the tables against the bit loop) on arm64, LLVM and the Go backend; bench/perf/hash twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`crc64.checksum` and `crc64.update` (spec/std.md 51). The Int holds Go's uint64 bits.
