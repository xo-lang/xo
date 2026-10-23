# scan.find returns an absolute offset, not one from `from`

Observed 2026-10-23 (xo-deepseek-2, adding `Str.replace_n`).

**Symptom**: `Str.replace_n` over the strsearch corpus died with SIGBUS
inside `obj.rs_append_bytes`, in the cases that replaced a second or later
match (a partial count on a text with more matches).

**Cause**: Go's `strings.Replace` loop computes `j += Index(s[start:], old)`,
an offset relative to `start`. `scan.find(s, old, from)` returns the offset
in `s`, not in `s[from..]`. Porting Go's line literally made the piece end
`start + absolute`, past the end of the text, so the append read and wrote
outside the string.

**Fix**: assign it, `j = scan.find(s, old, start)`, as
`std/native/llrt/strings.xo` `replace` already did.

A related trap in the same loop: `seq_size(b, j)` at `j == b.len()` reads one
byte past the text (Go's `DecodeRuneInString(s[start:])` gives a width of 0
there), so an empty `old` needs `if i > 0 && j < sl`.

**Read the crash**: a fault inside `obj.rs_append_bytes` called from a native
std function means one of its two ranges is out of order or out of bounds,
not that the allocator is at fault.
