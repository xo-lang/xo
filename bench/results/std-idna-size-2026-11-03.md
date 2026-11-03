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
| Go, file (`-s -w`, the fair comparison) | 1,657,842 | 1,890,626 | +232,784 | 1,757,938 | +100,096 |
| Xo, file | 232,675 | 399,747 | +167,072 | 299,155 | +66,480 |

By section (Go: `__text` code; tables are `__noptrdata` plus `__rodata`; `__gopclntab` is function and line metadata; the DWARF `__zdebug_*` sections are 47 KB of the idna delta and `-s -w` drops them and the symbol table, 41 KB in all. Xo: `__text` code, `__const` tables and constants):

| | Go idna adds | Xo idna adds | Go norm adds | Xo norm adds |
|---|---|---|---|---|
| code (`__text`) | +53,712 | +43,212 | +23,536 | +9,404 |
| table data | +134,466 (`__noptrdata` +104,834, `__rodata` +29,632) | +120,160 (`__const`) | +64,450 (`__noptrdata` +49,538, `__rodata` +14,912) | +58,248 (`__const`) |
| function metadata | +40,462 (`__gopclntab`) | none | +18,708 | none |

(idna includes norm, the Bidi table and the Bidi rule in both languages. The tables and code are measured by section; the Xo `__const` also holds a few KB of unrelated constants.)

## Verdict

- **std/norm: smaller than Go's**: +66 KB against +100 KB stripped (34 percent less; +121 KB with Go's debug sections), with code 2.5 times smaller (+9.4 KB against +23.5 KB) and table data 10 percent smaller (58.2 KB against 64.5 KB).
- **std/idna: smaller than Go's**: +167 KB against +233 KB stripped (28 percent less; +274 KB default), with code 20 percent smaller (+43 KB against +54 KB) and table data 11 percent smaller (120 KB against 134 KB, both including norm and the Bidi table).
- Speed: bench/results/std-twin-rows16-2026-11-03.md (norm is still slower than Go's, see there).

How it got there, from the first measurement of the same day (std-norm-size-2026-11-03.md: norm +292 KB; the first idna build: +300 KB): (1) norm's tables as byte records, not `List[Int]` constants (200 KB to 70 KB, and the list initializers' code went); (2) idna's joiner state table as a `Bytes` constant: a `List` constant is built by a lazy initializer at first use, which links in the runtime's task pool (the program grew by about 85 KB of runtime for one 48 element list); (3) no Unicode printing or case tables linked for an error message and an ASCII lower case; (4) runs of runes with the same delta stored once (idna's mappings, norm's compatibility decompositions); idna's infos (119 distinct) stored once and referenced by a byte.

## How each stores its tables, and why

Go's x/text and x/net use tries generated per Unicode version (`maketables`): a value block table (`idnaValues`, `nfcValues`, 2 bytes per entry), an index table that maps a rune's upper bits to a block (so blocks with equal values are stored once), sparse range lists for the rest, and a string of decompositions and mappings. A lookup is two or three array reads and no search. Equal blocks are shared, which is where the density comes from.

Xo's tables are flat sorted records found by binary search, written by `scripts/gennorm` and `scripts/genidna` (also from x/text's and x/net's own tables, so the data is the same):

- std/norm: 2,061 canonical and 3,812 compatibility decompositions as 7 byte records (a rune or a run of consecutive runes at one delta, then the delta or the position in a pool of varint runes), 388 combining class ranges (7 bytes) and 941 composition pairs (9 bytes): 57 KB (canonical 14.4 KB and 7.3 KB of pool, compatibility 17.0 KB and 7.5 KB, classes 2.7 KB, pairs 8.5 KB). Before the packing the same data was `List[Int]` constants at 8 bytes an element, 200 KB.
- std/idna: 7,380 ranges of runes of equal data, 7 bytes each (first rune, an index into 119 distinct infos of category, joining and attribute flags, Bidi class and mapping kind, and 3 bytes that hold the zigzag delta of a one rune mapping or the position of a longer one): 51.7 KB, plus 6.5 KB of mappings and 0.5 KB of infos; the Bidi class sits in the same info, so it needs no second table (Go has a 20 KB `bidiValues` beside the 46 KB of idna tables).

A binary search over fixed records is simple and needs no table generator beyond the dump, but stores a record for every run where Go's trie shares equal blocks.

## What is left

Both modules are now smaller than Go's, so the fix path is about margin and speed, not a deficit:

1. Norm's composition pairs (8.5 KB) are the inverse of the canonical two rune decompositions; storing only the decompositions and deriving the pair on lookup would drop them, at a cost in lookup time that has to be measured.
2. A two stage block table (an index of blocks, deduplicated blocks of values) for idna's 7,380 ranges would trade the binary search for two reads, as Go's trie does; worth it only if the timing rows show the search.
3. Speed: norm's NFC is 2.7 of Go's time on a Mac run after the byte packing (the record decoding), and was 2.1 before it on the host (bench/results/std-twin-rows12-2026-11-03.md); the host rows are in std-twin-rows16-2026-11-03.md.
