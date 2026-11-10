# 0595: Rune, Int and Time gaps from the std audit

Status: Accepted (owner, 2026-11-10, through the lead: the std audit's
top 40 after the ordered items)

## Context

bench/results/std-audit-2026-11-10b.md's top 40 lists utf8.RuneSelf
(36 stdlib files), unicode.MaxRune, unicode.SimpleFold, bits.Len,
bits.Add64 and Sub64, Time.UnixNano and Time.Nanosecond. Each is small;
together they are the rest of the prelude's numeric and time gaps.

## Decision

- `Rune.max` (U+10FFFF) and `Rune.max_ascii` (U+007F), type constants in
  0587's form. Go's RuneSelf (0x80) is `max_ascii + 1`: `b < RuneSelf`
  ports as `<= Rune.max_ascii`; a constant named after Go's would read
  oddly as a Rune.
- `r.simple_fold() -> Rune`: unicode.SimpleFold, the native
  unicode.simple_fold that std/regexp already uses.
- `n.bit_len() -> Int`: bits.Len64 on n's word.
- `n.add_carry(m, carry) -> (Int, Int)` and `n.sub_borrow(m, borrow) ->
  (Int, Int)`: bits.Add64 and Sub64 on unsigned words, the result and
  the carry or borrow out.
- `t.unix_micros()`, `t.unix_nanos()`, `t.nanosecond()`,
  `Time.from_unix_micros(us)`, `Time.from_unix_nanos(ns)`: Go's
  UnixMicro, UnixNano, Nanosecond, time.UnixMicro and time.Unix(0, ns).
  Micros round down, as Go's do for times before 1970.

## Consequences

- Native bodies in std/native/prelude.xo (Xo, no compiler change; Time's
  through `t - Time.UNIX_EPOCH`), Go's in std/rt/gap0595.go and
  intbounds.go. Golden rune_int_time.
- add_carry and sub_borrow are Go's generic formulas; an arm64 adds/adcs
  lowering is later work if a profile shows them.

## Alternatives

- **`Rune.self`**: Go's name, but `self` reads as a receiver.
- **Leave UnixNano as `(t - Time.UNIX_EPOCH).nanos()`**: works, but ports
  read worse and the audit counts it missing.
