# A word trick's shifts and sums fault in Xo, they wrap in Go

Status: Observed (2026-10-24, std/native/llrt the fold and replace work)

Xo's `+ - * <<` on Int are checked: they fault with the operands and the
position where C or Go would wrap. A SWAR loop ported from C therefore
faults on the very values it exists for:

```
fault: integer overflow: 169 << 56 does not fit Int
fault: integer overflow: ... + ... does not fit Int
```

Both were hit while folding ASCII case eight bytes at a time in
`obj.fold` and building a replacement word in `strings.word_of`.

Cause: a bit pattern with the top byte in use, or the top bit of an
8-byte word, is a negative Int; the compiler's range analysis can prove
most loop arithmetic safe but never a value read from memory.

Fix: use the wrapping forms where the wrap is the point, and note that a
word whose top byte is in play is a negative Int:

- `Int(b[k]).wrapping_shl(k * 8)` builds a little-endian word byte by
  byte;
- `w.wrapping_add(k)` and `w.wrapping_sub(k)` change the letter bytes of
  a word in place.

Two traps around the same code, both hit here:

- `n / 4` on a mask whose top bit is set is not `n >> 2` in unsigned
  arithmetic: 0x8000000000000000 divided by four is 0xE000000000000000
  (signed), not 0x2000000000000000. Shift the mask and clear the
  replicated sign bits instead: `(m >> 7) & 0x01FFFFFFFFFFFFFF`.
- A per-byte range test on a word must clear each byte's high bit before
  the adds (`w & 0x7F7F...`) or the sums carry into the next byte; and a
  byte at or above 0x80 has to be excluded from the result separately, or
  its low seven bits look like a letter.

Go compiles the same source without the checks and wraps silently; a
port's tests catch it only where a byte of the high kind appears, which
is why corpus/bytes' invalid UTF-8 rows found the first one.
