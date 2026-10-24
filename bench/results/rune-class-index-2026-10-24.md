# A two level index for the Rune class search: the classes row to 0.767, 2026-10-24

`unicode.in_ranges` binary searched a class's sorted range table (LETTER
has 657 ranges, 10 steps) once per rune at or above U+0800, and the
classes row measured 0.870 of the base after its overflow checks went
and `is_letter` folded its ASCII ranges (in-ranges-2026-10-24.md). The
search itself was the rest of the row. Each class now has a second
level: a 16 bit entry per 1,024 rune block naming the first range a
rune of that block can be in, so a lookup is one load and a scan of the
one or two ranges the block holds.

## The change

`internal/llvm/unicode_test.go` (TestUnicodeTables) emits, beside each
`<CLASS>` table, `<CLASS>_IX`: for each block of `1 << runeIXShift` runes
from U+0080, the number of the first range whose hi is at or above the
block's start, little endian, then the table's eight zero bytes of pad.

`std/native/llrt/unicode.xo` `in_ranges` takes the index table:

    let b = r.wrapping_sub(0x80).shr_logical(IX_SHIFT)
    var i = rt.bytes_word(ix, b.wrapping_mul(2)) & 0xffff
    while i < n {
      let w = rt.bytes_word(t, i.wrapping_mul(6))
      if r < (w & 0xffffff) { return false }
      if r <= (w.shr_logical(24) & 0xffffff) { return true }
      i = i.wrapping_add(1)
    }
    false

One load of the index, then per candidate one load holding the range's
lo in its low three bytes and its hi in the next three. `r >= 0x80` at
every call (the ASCII dispatch is the caller's), and the ranges are
sorted and disjoint, so a range whose lo is above r ends the scan.

`IX_SHIFT` (`runeIXShift` in the generator, which writes the tables) is
10: 1,024 runes a block, 1,088 blocks, 2,176 bytes a class, 26,208 bytes
for the twelve classes.

## What the table costs a program

The index ships in every program that calls a class method, one
`<CLASS>_IX` per class the program references. A program whose only
class work is `Rune.is_letter` ("ab€日本語𐐀", arm64) is 1,994,961 bytes
before and 1,994,959 after: its `__TEXT,__const` section grows by 2,200
bytes (LETTER_IX's 1,088 entries and their pad), and the file itself
does not grow, since the `__TEXT` segment is page padded (638,976
bytes on both). All twelve classes come to 26,208 bytes of `__const`.

## A/B, one revision, A = base, B = the change

`xo bench --ab`, 10 rounds, `bench/perf/unicode`, the same source built
by each xo, instructions only (wall rows wait for a quiet window).
B/A below 1.0 is better.

| block | index bytes | classes instructions, B/A | classes B |
|---|---|---|---|
| none (binary search) | 0 | 1.000 | 581.05 M |
| 4,096 (shift 12) | 6,528 | 0.831 | 482.90 M |
| 1,024 (shift 10) | 26,208 | 0.767 | 445.71 M |
| 512 (shift 9) | 52,224 | 0.726 | 422.36 M |

Against the previous change (`in-ranges-2026-10-24.md`, 505.47 M) the
shipped 1,024 block is 0.882. The `fieldsfunc` row, which is `is_space`
and never reaches `in_ranges`, measures 1.007 in every variant above,
so its cost is the layout the new code gives the split loop, not the
search. The classes checksum is identical to the base's on both
blocks measured (129,070,000).

Shift 9 buys another 4 points of the row for another 26 KB; the shipped
block keeps the static data at 26 KB, about a quarter of what the rest
of `unicode_tables.xo` costs.

## The first cut was wrong, and the checksum said so

The first version read the index at one byte per block instead of two,
so each entry was two neighbouring entries' bytes combined; it measured
1.401 of the base and the classes checksum fell from 129,070,000 to
126,750,000. The two byte offsets are `b * 2`, and the generator's
entry is `byte(j), byte(j >> 8)`.

## Coverage

The `runes`, `words`, `trimchars`, `regexp`, `strs` and `where` corpora
on arm64 and llvm, `TestUnicodeTables` (its staleness check regenerates
the tables and compares), the wasm goldens, and the full fixed point.
