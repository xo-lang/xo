# Std ports: Go's idioms that are free in Go cost in Xo

Observed 2026-10-21. Several native std modules ported from Go's packages
ran 3 to 10 times slower than Go's (bench/results/json-vs-go-2026-10-21.md,
bench/results/regexp-time-csv-vs-go-2026-10-21.md), while json, url,
mail and multipart matched or beat Go.

## Symptom

A port passes its Go-generated corpus (same output as Go) but its timing
row against Go's package is several times slower: regexp match 7.6x,
time format and parse 10x, base64 decode 3.4x natively, template
renders 6x.

## Cause

Two kinds, both fixed std-side:

1. **The port skipped Go's fast paths.** The first ports aimed at
   correct behavior and translated Go's general algorithm only:
   - regexp: Go's general Pike VM without its literal-prefix scan,
     onepass matcher and bit-state backtracker (porting two of them:
     7.6x to 2.1x);
   - template: re-parsed the template set on every render; Go parses
     once (a Template now keeps an opaque Parsed field);
   - base64/base32: a small list allocated per quantum; Go uses a
     256-byte table and a four- or eight-symbol fast path.
2. **A Go idiom that is free in Go costs in Xo:**
   - slicing a Str or Bytes copies (Go's slices share): a port of
     time's nextStdChunk that sliced the layout per field got slower
     (parse 3,768 to 6,208 M); Go compares bytes by index;
   - a piece List grown one push at a time: `Str.split("")`,
     `Str.split_n("")` and `Bytes.split("")`, one piece per rune, spent
     most of their time in `obj.rl_grow_n` and `alloc.resize` (split_empty
     841.6 M instructions, 40 ms); sizing once from the rune count as Go's
     explode does, and making each piece with `rt.str_view`, took it to
     327.2 M (0.39). Counting the pieces first for the *other* forms is not
     the same win: a list grows geometrically, so it copies O(pieces) words
     in all, while a count is a second walk over every byte (and, for
     fields, a second decode per field), and it measured slower: fields
     1.50x, bytes_fields 1.49x, lines 1.21x, split_after 1.10x, bytes_split
     1.10x, split_one 1.07x, all reverted
     (bench/results/strsplit-vs-go-2026-10-24.md, 2026-10-24);
   - small temporary Lists allocate (Go uses fixed-size arrays on the
     stack), e.g. `[0, 0, 0, 0]` per quantum in base64;
   - Xo has no package-level state, so what Go computes once per
     process can be recomputed per call;
   - a per-element write into a Builder costs a call chain in Xo, where
     Go's inlined `WriteByte` is nearly free: `Str.map_runes` pushed
     every rune through `raw.push_rune` and its runtime calls
     (`obj.rs_push_byte`, `obj.reserve`), half the mapping time, at 1.20
     and 1.38 times Go's instructions; keeping a rune that maps to
     itself as its own bytes and flushing runs with one
     `rt.rs_append_bytes` took it to 0.86 and 0.90
     (bench/results/maprunes-batch-2026-10-24.md, 2026-10-24).
   - Int arithmetic is checked in Xo (Go's wraps): an index scaled per
     iteration (`8 * i`) is a multiply with an overflow test and a fault
     branch each time, and `len % 4` on an Int that may be negative is a
     signed remainder (four instructions); bigmod's loops and mac_row's
     per-call `8 * (len - len % 4)` cost RSA sign4096 7.8% of its
     instructions (sign2048 11.7%) against byte offsets carried with
     `wrapping_add` and `wrapping_shl(3)` and a mask (2026-10-24).

## Fix

- Port Go's fast paths from the start, not only its general algorithm;
  when Go has a special case for speed, port that too.
- Index bytes rather than slicing strings in hot loops; use tables and
  builders written in place instead of small Lists.
- Append in runs, not per element: copy an unchanged span with one
  `rt.rs_append_bytes` instead of a `raw.push_rune` per rune.
- In a hot loop over words, carry the byte offset (`o = o.wrapping_add(8)`
  to `n.wrapping_shl(3)`) rather than computing `8 * i`, and mask a
  length known not negative (`end & -32`) rather than `% 4`.
- In a SWAR loop, choose the direction outside it (two loops, not a
  compare and a select per word), keep a byte mask as a mask:
  `letters.shr_logical(2)` is the 0x20 per letter that
  `((letters >> 7) & mask) * 32` takes three instructions for, and
  `between8(...) & (w ^ -1)` is one and-not where masking a masked value
  again is two instructions. obj.fold_copy lost five of 21 instructions
  per word to these (bench/results/fold-copy-2026-10-24.md).
- In a hot loop, take the checks out of arithmetic that cannot
  overflow: `lo.wrapping_add(hi).shr_logical(1)`,
  `m.wrapping_mul(6)` and `lo.wrapping_add(1)` in a binary search print
  as `add`, `lsr`, `mul`, `add`, where the plain forms print an
  `adds`/`b.vs` fault path and a signed division's `asr`;
  unicode.in_ranges went from 17 to 13 instructions an iteration
  (bench/results/in-ranges-2026-10-24.md). The ASCII letter fold
  `(r | 32) - 97 < 26` is NOT one of these: `-` is signed, so it prints
  as a checked `subs` plus a signed compare, and a control byte reads
  as a letter. Write the two sided test on the folded value
  (`x = r | 32` then `x >= 97 && x <= 122`), one unsigned compare, and
  put the `r >= 0x80` dispatch first so a non-ASCII rune pays nothing
  for it.
- Every port gets a timing row against Go's package next to its
  corpus (bench/results), so a slow port shows at once, not after a
  profile; profile with XO_PROF=cpu before rewriting.
- Call a run scanner only where a run can start. `unicode.quote` called
  `scan.span` at the top of every iteration, and for a byte that is not
  kept (a control byte, `"`, `\`, or the lead byte of a rune) span
  returns its `from` unchanged, so a run of Greek text paid one span
  call per rune. Testing the byte at `i` first and calling span only for
  a byte the loop keeps took the quote row from 97.8 M to 84.6 M
  instructions, 0.865 (bench/results/quote-decode-2026-10-24.md). The
  same profile says short runs do not pay for a wider step: a 16 byte
  `stops8` step in span cost 4.6% on that workload, because the runs of
  kept bytes are a few bytes long.
- Planned helpers: zero-copy Bytes views (design note), fixed-size
  arrays (language poll), and `xo vet --cost` notes for per-iteration
  copies and allocations.
