# Debug quoting into a fresh builder: a pre-sized builder, 13% fewer instructions, 2026-10-24

`text.append_quote` writes a Str's Debug text. It started from a builder
of capacity 16 (or the caller's) and doubled as it appended, so a Debug
of one string refilled the buffer three or four times. `unicode.quote`
already pre-sizes with `rt.rs_with_cap(n + 2)`; this does the same for
the Debug form when the builder it is given is empty.

## The change

std/native/llrt/text.xo `append_quote`: when `out.len() == 0`, the
builder is `rt.rs_with_cap(n + n/4 + 16)`, room for the text, a quarter
more for escapes and the quotes. A builder that already holds text is
used as it was, so the cost on nested Debug text is the one `len()` test.

## A/B, one revision, A = base, B = the change

`xo bench --ab`, 12 rounds, `bench/perf/text`, the same source built by
each xo. B/A below 1.0 is better.

| workload | A instructions | B instructions | B/A | A wall ms | B wall ms | B/A |
|---|---|---|---|---|---|---|
| quote (Debug of 1,000 Strs and 1,000 Bytes) | 128,276,759 | 111,924,742 | 0.873 | 7.977 | 7.163 | 0.898 |
| debug (nested records, every field) | 228,280,679 | 228,524,651 | 1.001 | 17.61 | 18.04 | within noise |

The nested workload's +0.1% is the guard: one `len()` test on each
append after the first string. The checksum of the quote workload is
identical on both sides (879119452).

## Coverage

The `template`, `strings`, `strconv`, `json`, `jsonencode` and
`jsondecode` corpora on arm64 and llvm (Debug text is an output these
compare), the wasm goldens (TestSelfhostWasm), and the full fixed point
(s1 = s2 = s3), since text.xo is in the fixed point set.
