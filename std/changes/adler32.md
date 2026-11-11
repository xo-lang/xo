# std/adler32 changes

## 2026-11-11: refactor: std/adler32's checksum is Xo on every backend (decision 0597 part 2)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 part 2 (merge std modules with their native code)
- Go reference: none (no behavior change)
- Test: TestStdCorpora adler32 on go, arm64, llvm
- Shipped in: not yet released

checksum is `update(1, data)` in std/adler32.xo on every backend. update
stays the fast path: bodyless, std/native/adler32_native.xo's word loop on
the native backends (a byte by byte loop measured 1.085x the time), Go's
hash/adler32 on the Go backend.

## 2026-11-03: port: std/adler32, Go's hash/adler32

- Kind: port
- Commit: this commit
- Found by: the lead's roadmap (priority 2: the hash packages), std-coverage.md planned row
- Go reference: hash/adler32 (adler32.go: two sums mod 65521, nmax blocks) of go1.26.1
- Test: internal/stdtest/testdata/corpus/adler32 (Go's values: its golden strings and pseudo random buffers of lengths that cross the word step and the block, split at several points) on arm64, LLVM and the Go backend; bench/perf/hash twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`adler32.checksum` and `adler32.update` (spec/std.md 52). With 64 bit sums the modulus is taken once a megabyte, not every 5,552 bytes.
