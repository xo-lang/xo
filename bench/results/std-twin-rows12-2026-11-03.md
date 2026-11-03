# Timing rows against Go, part 12 (2026-11-03)

Linux/arm64 offload host, `xo bench --ab` style runs (15 rounds, Go and Xo alternated, caches off) of bench/perf/cookiejar: `cookiejar` (decision 0475) against Go's net/http/cookiejar, and bench/perf/norm: `norm.normalize` (decision 0476) against golang.org/x/text v0.43.0's unicode/norm. For cookiejar each round stores three cookies for one of 40 URLs and asks for the cookies of a deeper URL, 100000 rounds.

| bench | workload | Go wall (ms) | Xo wall (ms) | Xo / Go | interval | peak memory |
|---|---|---|---|---|---|---|
| cookiejar | none | 0.862 | 0.515 | 0.598 | 0.547-0.641 | 0.344 |
| cookiejar | set | 73.57 | 122.6 | 1.666 | 1.646-1.679 | 0.215 |
| cookiejar | get | 114.5 | 176.3 | 1.54 | 1.523-1.556 | 0.212 |
| cookiejar | both | 203.8 | 342.5 | 1.681 | 1.666-1.691 | 0.221 |
| norm | none (setup: build the 600000 character text) | 4.078 | 6.36 | 1.56 | 1.478-1.593 | 0.565 |
| norm | ascii (20 times) | 7.269 | 8.48 | 1.167 | 1.153-1.377 | 0.712 |
| norm | nfc (20 times) | 85.7 | 485.8 | 5.669 | 5.611-5.869 | 2.035 |
| norm | nfd (20 times) | 120.0 | 250.9 | 2.09 | 2.077-2.165 | 1.623 |
| norm | nfkc (20 times) | 132.0 | 507.5 | 3.844 | 3.751-3.987 | 2.031 |

Not Proven: 1.5 to 1.7 of Go's wall, with a fifth of Go's peak memory. Likely cost (not yet profiled): every call builds the host key and entry ids as strings, lower cases the host, and the jar's maps are keyed by string; Go does the same work but with its tuned map and no per call list copies. Next: profile `set` and `get` and cut the allocations.

Not Proven, std/norm: NFC is 5.7 of Go's wall, NFKC 3.8, NFD 2.1, and the ASCII fast path 1.17 (most of it the scan of the text). The measured text is 90 percent ASCII, where x/text copies runs that need no work and only decomposes the rest, with a trie lookup. Here every rune goes through a `List[Int]` of decomposed runes, a binary search of the tables, the reorder pass and the composition pass, and the result is rebuilt rune by rune. Next: (1) a quick check scan that copies the longest prefix of runes below 0xC0 (or with no decomposition and class 0) as one slice and starts the work at the first rune that needs it, (2) a lookup array by rune block in place of the binary search, (3) write the result with byte slices instead of a StrBuilder per rune, (4) 4 byte element tables (the data is about 200 KB).
