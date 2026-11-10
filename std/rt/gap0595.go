package rt

import (
	"math/bits"
	"unicode"
)

// Decision 0595: the std audit's Int, Rune and Time gaps.

// Int_bit_len is `n.bit_len()`: bits.Len64.
func Int_bit_len(_ *Task, n int64) int64 { return int64(bits.Len64(uint64(n))) }

// Int_add_carry is `n.add_carry(m, carry)`: bits.Add64.
func Int_add_carry(_ *Task, n, m, carry int64) Tuple2[int64, int64] {
	s, c := bits.Add64(uint64(n), uint64(m), uint64(carry))
	return Tuple2[int64, int64]{int64(s), int64(c)}
}

// Int_sub_borrow is `n.sub_borrow(m, borrow)`: bits.Sub64.
func Int_sub_borrow(_ *Task, n, m, borrow int64) Tuple2[int64, int64] {
	d, b := bits.Sub64(uint64(n), uint64(m), uint64(borrow))
	return Tuple2[int64, int64]{int64(d), int64(b)}
}

// Rune_simple_fold is `r.simple_fold()`: unicode.SimpleFold.
func Rune_simple_fold(_ *Task, r Rune) Rune { return Rune(unicode.SimpleFold(rune(r))) }

// Time_unix_nanos is `t.unix_nanos()`.
func Time_unix_nanos(_ *Task, t Time) int64 { return int64(t) }

// Time_unix_micros is `t.unix_micros()`, rounded down.
func Time_unix_micros(_ *Task, t Time) int64 { return floorDiv(int64(t), 1e3) }

// Time_nanosecond is `t.nanosecond()`.
func Time_nanosecond(_ *Task, t Time) int64 { return int64(t) - floorDiv(int64(t), 1e9)*1e9 }

// Time_from_unix_nanos is `Time.from_unix_nanos(ns)`.
func Time_from_unix_nanos(_ *Task, ns int64) Time { return Time(ns) }

// Time_from_unix_micros is `Time.from_unix_micros(us)`.
func Time_from_unix_micros(_ *Task, us int64) Time { return Time(MulS(us, 1e3, "")) }
