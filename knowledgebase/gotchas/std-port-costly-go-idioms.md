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
   - small temporary Lists allocate (Go uses fixed-size arrays on the
     stack), e.g. `[0, 0, 0, 0]` per quantum in base64;
   - Xo has no package-level state, so what Go computes once per
     process can be recomputed per call.

## Fix

- Port Go's fast paths from the start, not only its general algorithm;
  when Go has a special case for speed, port that too.
- Index bytes rather than slicing strings in hot loops; use tables and
  builders written in place instead of small Lists.
- Every port gets a timing row against Go's package next to its
  corpus (bench/results), so a slow port shows at once, not after a
  profile; profile with XO_PROF=cpu before rewriting.
- Planned helpers: zero-copy Bytes views (design note), fixed-size
  arrays (language poll), and `xo vet --cost` notes for per-iteration
  copies and allocations.
