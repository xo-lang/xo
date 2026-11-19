# Int has wrapping_shl but no wrapping_shr

Status: Observed (2026-10-23), writing Str.find_any's set lookup.

**Symptom:** `value.wrapping_shr(k)` is `XO0601: Int has no field or
method wrapping_shr` (and `XO1201: not available on the native backends
yet: this call`), though `wrapping_shl` exists.

**Cause:** the prelude names the two asymmetrically: `wrapping_shl` is the
left shift that drops the bits shifted out (std/prelude.xo 184), while the
64 bit right shift with zeros at the top is `shr_logical` (187), because
`>>` keeps the sign.

**Fix:** use `n.shr_logical(k)`; std/native/llrt/scan.xo keeps `shr(a, k)`
and `shl(a, k)` for the same reason. Both shift by `k mod 64`, so a bit
sample taken from a byte (0 to 127) needs its own bound before the shift,
not a shift that saturates.

Observed again 2026-11-18 (task #383): porting Go's math.RoundToEven, the
line `bits += (halfMinusULP + (bits>>(shift-e))&1) >> e` relies on Go's
unsigned shifts past 63 bits giving 0, so an exponent of 52 or more (whole
numbers, Inf, NaN) leaves the bits alone. The direct port with
`shr_logical` shifted by `k mod 64` and turned NaN into -0.0 and 1e300
into 9.99999999995523e+299. Fix: return x before the shifts when the
unbiased exponent is 52 or more. When porting a Go shift by a computed
count, check what Go gives past 63 and guard it.
