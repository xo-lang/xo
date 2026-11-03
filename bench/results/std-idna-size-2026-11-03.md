# std/idna and std/norm: what importing them costs, beside Go's (2026-11-03)

The owner's question (decision 0476): is our method better than Go's? This file measures the size of a program that imports golang.org/x/net/idna (which pulls in x/text's unicode/norm, unicode/bidi and secure/bidirule) and one that imports golang.org/x/text/unicode/norm, against the same programs in Xo, darwin/arm64, built the same way as std-norm-size-2026-11-03.md (that file's norm figures were measured before the packing below and are superseded by this one). Xo at the commit that adds this file (std/norm and std/idna as pushed with it); Go go1.26.1, golang.org/x/net v0.60.0, golang.org/x/text v0.43.0, both with Unicode 15.0.0 tables.

## Programs and commands

Go (`go.mod` requiring x/net v0.60.0 and x/text v0.43.0; each program prints its first argument and its length, B and C also call the package):

```go
// goa: no import
fmt.Println(s, len(s))
// gob: golang.org/x/net/idna
a, _ := idna.Lookup.ToASCII(s)
u, _ := idna.Lookup.ToUnicode(s)
fmt.Println(a, u, len(s))
// goc: golang.org/x/text/unicode/norm
fmt.Println(norm.NFC.String(s), len(s))
```

```sh
GOFLAGS=-mod=mod GOPROXY=off go build -o goX.bin .
GOFLAGS=-mod=mod GOPROXY=off go build -ldflags="-s -w" -o goX.stripped .
```

Xo (`xo.mod` of `module example.com/xN` and `edition preview`):

```xo
// xa: no import
os.stdio.println("${s} ${s.len()}")
// xb: use std/idna
let a = idna.to_ascii(s).ok() ?? ""
let u = idna.to_unicode(s).ok() ?? ""
os.stdio.println("${a} ${u} ${s.len()}")
// xc: use std/norm
os.stdio.println("${norm.normalize(s, norm.Form.Nfc)} ${s.len()}")
```

```sh
XO_STD=$CHECKOUT/std $CHECKOUT/bin/xo build --release -o xN.bin .
size -m X.bin     # sections summed by name
```

## Sizes (bytes)

| | no import | imports idna | idna adds | imports norm only | norm adds |
|---|---|---|---|---|---|
| Go, file (default build, with DWARF) | 2,492,450 | 2,766,354 | +273,904 | 2,613,122 | +120,672 |
| Go, file (`-s -w`) | 1,657,842 | 1,890,626 | +232,784 | 1,757,938 | +100,096 |
| Xo, file | 232,675 | 532,819 | +300,144 | 315,715 | +83,040 |

By section (Go: `__text` code; tables are `__noptrdata` plus `__rodata`; `__gopclntab` is function and line metadata; the DWARF `__zdebug_*` sections are 47 KB of the idna delta and `-s -w` drops them and the symbol table, 41 KB in all. Xo: `__text` code, `__const` tables and constants):

| | Go idna adds | Xo idna adds | Go norm adds | Xo norm adds |
|---|---|---|---|---|
| code (`__text`) | +53,712 | +118,116 | +23,536 | +9,492 |
| table data | +134,466 (`__noptrdata` +104,834, `__rodata` +29,632) | +164,760 (`__const`) | +64,450 (`__noptrdata` +49,538, `__rodata` +14,912) | +70,392 (`__const`) |
| function metadata | +40,462 (`__gopclntab`) | none | +18,708 | none |

(idna includes norm, the Bidi table and the Bidi rule in both languages.)

## Verdict

- **std/norm: smaller than Go's** as a whole program (+83 KB against +100 KB stripped, +121 KB with Go's debug sections): code is 2.5 times smaller (+9.5 KB against +23.5 KB), table data 9 percent larger (70.4 KB against 64.5 KB).
- **std/idna: larger than Go's**: +300 KB against +233 KB stripped (+274 KB default), 29 percent more. The tables are 22 percent larger (164.8 KB against 134.5 KB, both including norm) and the code 2.2 times larger (+118 KB against +54 KB); the code excess is the larger part of the gap.
- Speed: bench/results/std-twin-rows15-2026-11-03.md.

## How each stores its tables, and why

Go's x/text and x/net use tries generated per Unicode version (`maketables`): a value block table (`idnaValues`, `nfcValues`, 2 bytes per entry), an index table that maps a rune's upper bits to a block (so blocks with equal values are stored once), sparse range lists for the rest, and a string of decompositions and mappings. A lookup is two or three array reads and no search. Equal blocks are shared, which is where the density comes from.

Xo's tables are flat sorted records found by binary search, written by `scripts/gennorm` and `scripts/genidna` (also from x/text's and x/net's own tables, so the data is the same):

- std/norm: 2,061 canonical and 3,812 compatibility decompositions as 7 byte records (rune, count, position in a pool of varint runes), 388 combining class ranges (7 bytes) and 941 composition pairs (9 bytes): 70 KB. Before the packing the same data was `List[Int]` constants at 8 bytes an element, 200 KB (std-norm-size-2026-11-03.md); packing as bytes took the table from 200 KB to 70 KB and the code from +87 KB to +9.5 KB (the list initializers were code).
- std/idna: 7,380 ranges of runes of equal data, 9 bytes each (first rune, category, joining and attribute flags, Bidi class, position of the mapping), 66 KB, plus 17 KB of mappings; the Bidi class sits in the same record, so it needs no second table (Go has a 20 KB `bidiValues` beside the 46 KB of idna tables). Idna's own table is 84 KB against Go's idna and bidi tables of about 66 KB.

A binary search over fixed records is simple and needs no table generator beyond the dump, but stores a record for every run where Go's trie shares equal blocks.

## Where Xo is worse and the fix path

1. idna code, +118 KB against +54 KB: not yet investigated. Candidates: the generic container machinery that each `Result`, `Option`, `List[Str]` and tuple instantiation brings in, string interpolation, and the label iterator's `List[Str]` helpers, none of which Go's idna needs. Next experiment: `xo build --release` with a symbol size listing (`nm -S` on the Mach-O) of xb.bin to rank idna's functions, then write the hot ones over bytes.
2. idna tables, 164.8 KB against 134.5 KB: a two stage table (an index of 1,088 blocks of 1,024 runes, then deduplicated blocks of 3 byte values) would store equal blocks once like Go's trie; expected 45 to 50 KB for idna, from the 66 KB of ranges and the 9 byte records. The mappings can be stored once with norm's compatibility decompositions where the two agree: idna's mapping is NFKC and case folding of the rune, so the mapped runes that are only a compatibility decomposition (how many of idna's mapped ranges, to be counted first) could be derived from std/norm's table instead of stored; this needs a casefold step that std/unicode has.
3. norm table data, 70 KB against 64 KB: the same two stage layout for the decomposition index, or 2 byte entries for runes below 0x10000 in the pools.

The data is correct on every backend (corpora against x/text and x/net, bench/results/std-twin-rows12 and 15 for speed), so these are size and speed work, not correctness.
