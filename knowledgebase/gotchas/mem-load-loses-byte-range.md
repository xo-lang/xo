# Reading a byte with rt.mem_load loses its range, and its arithmetic gets overflow checks

Status: Observed (2026-10-24, Xo Lang 1)

## Symptom

Replacing `Int(b[i]) - 48` with `rt.mem_load(p, i, 1) - 48` in std/native
llrt text.xo's num2, to drop a bounds check a digit (parse_time's length
test already covered the fields), made time_parse 8% more instructions,
not fewer. parse_time went from 678 instructions with 6 overflow checks
to 1,695 with 86.

## Cause

`b[i]` is a Byte, 0 to 255, and the native backends' range analysis
carries that through `Int(...) - 48`, `c0 * 10 + c1` and the rest, so it
proves those operations cannot overflow and emits no check.
`rt.mem_load(p, i, 1)` returns an Int of unknown range (the size argument
is not a type), so every product and sum after it is checked.

## Fix

Mask the load: `(rt.mem_load(p, i, 1) & 255) - 48` gives the range back
(9 overflow checks; the and costs nothing after a zero extending ldrb).
Before swapping an index for a raw load to save a bounds check, count the
overflow checks in the function's assembly (`grep -c
fault_raise_overflow`) before and after, and measure with xo bench --ab:
in this case, even with the mask, the change was 1.005 the instructions
(no gain), so it was dropped.

Better (2026-10-24, later the same day): make the arithmetic on the byte
wrap (`wrapping_sub(48)`, `(9).wrapping_sub(c)`, `wrapping_mul(10)`)
where the function's own checks bound it; then the lost range costs
nothing and no mask is needed. In parse_time_at the mask was not free
after all: an `and` a byte, 1.3% more instructions than the unmasked
wrapping form. The whole treatment (one span check, bytes read through
`raw.bytes_addr` taken once, wrapping arithmetic) took the JSON Time
decode to 0.783 of main's instructions and 0.884 of LLVM's, with 11
overflow checks where main had 29 (the earlier unmasked swap had 86).
