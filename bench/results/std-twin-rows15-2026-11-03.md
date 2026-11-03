# Timing rows against Go, part 15 (2026-11-03)

Linux/arm64 offload host (the first, exclusive), `xo bench --ab` with Go's
twin as A and Xo as B (15 rounds, 1 warmup, caches off) of
bench/perf/bytestransform at 9e2d98dfc: `Bytes.to_str_lossy` against
`utf8.Valid` then `string([]rune(string(b)))`, and `Bytes.to_valid_utf8`
against `strings.ToValidUTF8`, 200,000 calls over six texts (one with
invalid UTF-8) and three replacements. `index` is the loop alone. Go
1.26.1. Raw output and script: std-twin-rows15-2026-11-03/.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| bytestransform | index | 0.913 | 0.856 | 0.938 | 0.907-1.009 | 0.215 |
| bytestransform | lossy | 4.776 | 4.631 | 0.970 | 0.952-0.986 | 0.333 |
| bytestransform | valid | 7.218 | 4.895 | 0.678 | 0.669-0.687 | 0.121 |

Both are Proven better: `to_valid_utf8` 0.68 and `to_str_lossy` 0.97 of
Go's wall. The first version of `to_str_lossy` (c38bbf9a9, twin-r13.txt)
ran its own `to_str` check and a decode loop and measured 1.104
(1.077-1.148, worse); it now calls the runtime's conversion of outside
bytes (`scan.text_of`, decision 0259: a 16 byte ASCII skip, then the fix
up), which is what `Fs.read_text` already used (twin-r13b.txt).
