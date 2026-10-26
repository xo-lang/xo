# A value's range stops at a call, a loop counter, and a plain mask

Status: Observed (2026-10-25, std/native/llrt/text.xo `append_time`).

## Symptom

Formatting a `Time`'s nanoseconds (`text.append_time`'s `ns != 0` branch)
carried ten checks in the arm64 assembly, each a `cmp` and a branch to an
out of line `fault_raise_*`: the `k -= 1` countdown of the trailing zero
loop, `r = k - j`, `pow10`'s `p *= 10`, `digits_word(x / q, j) << 32`, the
`sw << (8 * k)` and its shift amount inside `cat`, `r + 1`,
`(4 + j) << 8` and `(r + 1) << 12`. Counted by hand in the assembly the
function is 703 lines; the same rules applied as masks and branches take
it to 543 with no check left, and `time_fmt` from 152,186,300 to
142,235,040 instructions retired (0.934,
bench/results/time-fmt-tail-2026-10-25.md).

## Cause

The value ranges the backends' check removal reads come from what the SSA
builder proved for the value itself, and three shapes lose it:

- A function's result has no range at its call site: `let q = pow10(r)`
  left q unknown, so `x / q` and `x % q` both checked for zero even when
  `pow10` returned one of ten constants. The same tree written at the call
  site proves 1 to 10,000,000.
- A loop counter bounds nothing by itself: `var k = 9` and `k -= 1` is a
  checked subtract, and `k`'s range at the loop exit is unbounded below.
  `k = (k - 1) & 0xF` both keeps the value (the loop runs at most 8 times)
  and gives the mask's range.
- `var j = 4; if k < j { j = k }` used as a subtraction and a shift amount
  is unchecked only once `j` and `r` have mask ranges of their own
  (`& 7`: j is at most 4, r at most 5, so the shift amounts stay below 64
  and the shifted values fit).

`digits_word`'s own masks bound its loop, not its result: `<< 32` on the
result still checked until the result was masked (`& 0x7FFFFFFF`).

## Fix

Write the bounded value so the range pass sees it: a mask (`& 0xF`, `& 7`,
`& 0x7FFFFFFF`), a branch whose arms refine the range, or a constant tree
written where it is used. Each mask is one `and`; each check it removes is
a compare and a branch plus a fault path. Count the checks in the
assembly before and after (`xo asm` on the function, `fault_raise_*`
labels twice: their jump sites and their bodies), and keep the outputs
identical: a format sweep compared against the before std is cheap and
caught nothing here, unlike a rewrite by hand (a first attempt derived the
digit count with `ndigits(x)` and would have printed `ns = 12000000` as
".12" where the 9 digit padded fraction gives ".012").

Related: [mem-load-loses-byte-range.md](mem-load-loses-byte-range.md) for
the same shape at a raw load, and
[xo-checked-word-arithmetic.md](xo-checked-word-arithmetic.md) for the
wrapping forms.
