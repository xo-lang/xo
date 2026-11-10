# A Go backend twin's enum tags start at 1

Status: Observed (2026-11-09, std/compress lzw_encode)

Symptom: `compress.lzw_encode(b"", compress.LzwOrder.LSB, 2)` gave 0x2c on
the arm64 and LLVM backends, as Go does, and 0x94 on the Go backend (the
MSB packing of the same codes); the lzw corpus failed on the Go backend
only.

Cause: a std function with a Go twin in std/rt receives an enum as
`Compress_LzwOrder{Tag, P}` whose first variant has `Tag` 1, not 0 (the
twin's own `Compress_Format` table is `{1: "gzip", 2: "flate", 3: "zlib"}`).
The twin tested `Tag == 0` for LSB, so every call took the MSB branch.

Fix: number the variants from 1 in the twin (`o.Tag == 1` for the first),
and run the new function's corpus on the Go backend: the native backends
cannot show this.
