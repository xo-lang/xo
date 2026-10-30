# std/crc32 changes

## 2026-10-29: perf: IEEE with x86-64's PCLMULQDQ and VPCLMULQDQ

- Kind: perf
- Commit: this commit
- Found by: bench/perf/hash crc32 at 8.084x Go's wall on linux/amd64
- Go reference: hash/crc32 crc32_amd64.s ieeeCLMUL (the SSE path; its AVX-512 path at 256 bits), crc32_amd64.go archUpdateIEEE
- Test: golden crc32_sums (IEEE at 64 to 12,345 bytes, word aligned and not, against a bit loop) on the owner's amd64 host, arm64, LLVM, Go parity and wasm; compiler/macho enc_amd64_test (pextrd, pcmpeqb, pand, pclmulqdq, and the ymm vpclmulqdq, vinserti128, vextracti128, vmovdqu forms); test/codegen/intrinsics flate.crc32 on amd64
- Timing: on the owner's linux/amd64 host (Ryzen 9 9955HX), bench/perf/hash crc32 --ab against Go's hash/crc32, 15 rounds: wall 0.976x Go (0.921 to 1.011, within noise), was 8.084x; the SSE fold alone 1.352x (1.277 to 1.403), where Go takes its AVX-512 path on this CPU

`Poly.Ieee` (and gzip's and zip's CRC-32, flate.crc32) folds 64 bytes a
step with PCLMULQDQ on x86-64 where the CPU has it (hw()'s HW_CLMUL), for
inputs of 64 bytes and more, then the last 15 or fewer bytes a word and a
byte at a time as before. From 128 bytes, a CPU with VPCLMULQDQ and AVX2
(hw()'s new HW_VPCLMUL, CPUID.7 ECX bit 10) folds the four lanes on two
256 bit registers.

## 2026-10-29: perf: Castagnoli in three interleaved streams

- Kind: perf
- Commit: this commit
- Found by: bench/perf/hash crc32c at 1.743x Go's wall on linux/amd64 after crc32q
- Go reference: hash/crc32 crc32_amd64.go archUpdateCastagnoli, castagnoliSSE42Triple, castagnoliShift (K1 168, K2 1344)
- Test: golden crc32_sums (lengths around 3 * 168 and 3 * 1344, word aligned and not, against a bit loop) on arm64, LLVM, Go parity, wasm and the owner's amd64 host
- Timing: on the owner's linux/amd64 host, bench/perf/hash crc32c --ab against Go's hash/crc32, 15 rounds: wall 0.846x Go (0.83 to 0.893), was 1.743x with one crc32q chain; darwin/arm64 quiet --ab against the parent (one crc32cx chain), 15 rounds: crc32c wall 0.479 (0.472 to 0.492), instructions 0.535 (was 0.992x Go)

Past 504 bytes, `Poly.Castagnoli` runs three blocks of K bytes at once
(K 1,344, then 168), each its own crc32q or crc32cx chain, so the
instruction's latency overlaps, and joins them with Go's shift tables
(the state carried over K zero bytes, 4 x 256 entries each).

## 2026-10-29: perf: Castagnoli with x86-64's SSE4.2 crc32q

- Kind: perf
- Commit: this commit
- Found by: the crc32cx follow-up (amd64 ran slicing-by-8)
- Go reference: hash/crc32 crc32_amd64.go castagnoliSSE42
- Test: compiler/macho enc_amd64_test (crc32q encodings), test/codegen/intrinsics crc32.castagnoli on amd64, the crc32 corpus and cpu_test under Docker linux/amd64
- Timing: on the owner's linux/amd64 host, bench/perf/hash --ab against Go's hash/crc32, 15 rounds: crc32c wall 1.743x Go (1.718 to 1.78), one crc32q stream against Go's three interleaved (castagnoliSSE42Triple), next; crc32 (IEEE) 8.084x Go (7.996 to 8.29), slicing-by-8 against Go's PCLMULQDQ folding, unchanged here

`Poly.Castagnoli` runs a word a step on `crc32q` where the CPU has SSE4.2
(hw()'s new HW_CRC32C); one chain, not Go's three-way interleave for
buffers past a few KiB.

## 2026-10-29: perf: Castagnoli with arm64's crc32cx

- Kind: perf
- Commit: this commit
- Found by: bench/perf/hash crc32c at 4.761x Go's wall
- Go reference: hash/crc32 crc32_arm64.go castagnoliUpdate (CRC32CX a word a step)
- Test: std/native/llrt/cpu_test.xo "crc32cx is its body" (arm64 and LLVM), the crc32 corpus on arm64, LLVM and Go, compiler/macho's encoding table, test/codegen/intrinsics crc32.castagnoli
- Timing: bench/perf/hash crc32c, quiet --ab 15 rounds: wall 0.206 (0.203 to 0.208) of the parent's, 0.992 (0.989 to 1.007) of Go's

`Poly.Castagnoli` runs a word a step on arm64's CRC32CX where the CPU
has the CRC32 extension (decision 0231 addendum, 2026-10-29); x86-64 and
older CPUs keep slicing-by-8.

## 2026-10-29: port: std/crc32, Go's hash/crc32

- Kind: port
- Commit: this commit
- Found by: the lead's queue (2026-10-29): a public CRC-32 for file formats and storage checksums; gzip and zip had it inside std/native/llrt/flate.xo only
- Go reference: hash/crc32 (crc32.go, crc32_generic.go: ChecksumIEEE, Checksum, Update, MakeTable(Castagnoli), slicing8Table) of go1.26.1
- Test: internal/stdtest/testdata/corpus/crc32 (crc32_test.go's golden inputs and more, values from Go's package; update split at every point; both tables against the bit loop for lengths 0 to 300) on arm64, LLVM and the Go backend; bench/perf/hash twincheck
- Timing: bench/perf/hash, quiet --ab 15 rounds against Go's hash/crc32 (darwin/arm64): crc32 (2,000 checksums of 64 KiB) wall 0.969x Go (0.955 to 0.976); crc32c wall 4.761x Go (4.722 to 4.781), software slicing-by-8 against Go's CRC32C instructions

`crc32.checksum(data, poly)` and `crc32.update(crc, data, poly)` with
`Poly.Ieee` (the default) and `Poly.Castagnoli`. IEEE runs on flate.xo's
slicing-by-8, with arm64's crc32x where the CPU has it; Castagnoli on
slicing-by-8 over its own generated table, in software (Go uses the CPU's
CRC32C instructions on arm64 and amd64: next, a crc32cx intrinsic).
