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
