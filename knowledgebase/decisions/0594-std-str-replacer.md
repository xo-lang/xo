# 0594: Replacer, Go's strings.Replacer

Status: Accepted (owner, 2026-11-10, through the lead: the std audit's
gaps, strings.NewReplacer fourth in the owner's order)

## Context

bench/results/std-audit-2026-11-10b.md lists strings.NewReplacer in its
top 40 (5 of 5 corpus modules). Xo had only `Str.replace` per pair, and
chaining it is not the same: a later pair rescans what an earlier one
wrote (escaping `&` after `<` turns `&lt;` into `&amp;lt;`), and each
pair is another pass over the text.

## Decision

A prelude type beside StrBuilder:

```
struct Replacer {}
fn (static: Replacer) new(pairs: List[(Str, Str)]) -> Replacer
fn (r: Replacer) replace(s: Str) -> Str
```

Go's semantics exactly: one pass; at each position the first pair in the
order given whose old starts there is replaced and the scan continues
after it; an empty old matches at every byte position and at the end,
but not twice in a row at one position. `replace` returns s itself when
nothing changes.

Pairs are a list of tuples rather than Go's flat argument list, so an
odd count (Go's panic) cannot be written.

new picks Go's algorithm, as NewReplacer does: one pair with an old over
a byte one pass of the native Index (singleStringReplacer); single bytes to single
bytes a 256 entry table (byteReplacer); single bytes to strings a table
of pairs (byteStringReplacer); any other the generic one. Go's generic
replacer walks a trie for the highest priority key that is a prefix at
the position; the native port groups the pairs by first byte, in order,
and takes the first that matches, which is the same pair, and skips
bytes no pair starts with (Go's fast path). The Go backend's runtime
wraps strings.Replacer.

## Consequences

- Prelude: Replacer; native in std/native/prelude.xo (replacer_*), Go
  in std/rt/replacer.go. Golden replacer: replace_test.go's TestReplacer
  cases over ASCII on every backend.
- Timing (Mac, 1 MiB, us per call, native against Go): byte to string
  1210 against 1060, generic 1800 against 2430, byte to byte 560 against
  660, one pair 650 against 760. The one pair case makes one pass with a
  growing buffer (Go's singleStringReplacer) where Str.replace counts
  first (1150; Go's strings.ReplaceAll 1660). Replacements of 1 to 8
  bytes are pushed as one word. Byte to string is the one behind Go:
  Go counts the new size first and copies into an exact buffer.
- Replacer.WriteString (to an io.Writer) has no counterpart; `replace`
  then a write covers it.

## Alternatives

- **`Str.replace_all(pairs)`**: no value to keep, so every call would
  rebuild the tables; Go code builds a replacer once and reuses it.
- **A std module**: Go's strings is Xo's prelude Str, and a replacer is
  as common as Str.replace.
