# One printable bitmap for the BMP: classify 9.5% and quote 1.2% fewer instructions, 2026-10-24

`unicode.is_print` (strconv.IsPrint) answered the runes U+0080 to U+07FF
from `PRINT_BITS` and the runes U+0800 to U+FFFF from `PRINT_BMP`, two
bitmaps and two range tests in front of them. One bitmap now covers
U+0080 to U+FFFF, so the second test is gone, and `is_print` fits the
arm64 inliner's "few" size (87 instructions, 21 blocks, 2 sites): the
calls inside loops (strconv's quoting, the class walks) get the body
inlined instead of the call.

## The change

`internal/llvm/unicode_test.go` generates `PRINT_BITS` over U+0080 to
U+FFFF (8192 bytes, the same total as the 192 and 7936 bytes it
replaces) and no longer writes `PRINT_BMP`;
`std/native/llrt/unicode.xo` `is_print` reads it after its ASCII test and
keeps the ranges search for the astral planes.

## A/B, one revision, A = base, B = the change

`xo bench --ab`, 10 rounds, the same source built by each side with one
xo binary and its own std. B/A below 1.0 is better.

| workload | A instructions | B instructions | B/A |
|---|---|---|---|
| unicode classify | 138,537,999 | 125,441,380 | 0.905 |
| unicode quote | 84,847,137 | 83,811,946 | 0.988 |
| unicode classes | 434,538,388 | 435,196,233 | 1.002 |
| text quote | 112,389,532 | 112,265,931 | 0.999 (within noise) |

`classes` is the one row that does not move: its `is_print` call is not
inside a loop, so that site still calls (1.002, 0.2% more, deterministic).
Every checksum matched the base's.

## Coverage

`TestUnicodeTables`, the `runes` corpora on arm64 and llvm, `stdfuzz`
area `runes` at seeds 1 to 3, the wasm goldens and the full fixed point.
