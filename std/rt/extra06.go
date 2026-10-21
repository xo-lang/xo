package rt

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
)

// Std additions from the stage 2 benchmark (std.md 0.6).

// Bytes_to_hex is `b.to_hex()`: lowercase hexadecimal.
func Bytes_to_hex(_ *Task, b Bytes) string { return hex.EncodeToString([]byte(b)) }

// Bytes_to_base64 is `b.to_base64()`: standard encoding with padding.
func Bytes_to_base64(_ *Task, b Bytes) string { return base64.StdEncoding.EncodeToString([]byte(b)) }

// Bytes_from_hex is `Bytes.from_hex(s)`: None if s is not valid hexadecimal.
func Bytes_from_hex(_ *Task, s string) Option[Bytes] {
	d, err := hex.DecodeString(s)
	if err != nil {
		return None[Bytes]()
	}
	return Some(Bytes(d))
}

// Bytes_from_base64 is `Bytes.from_base64(s)`: None if s is not valid
// standard base64.
func Bytes_from_base64(_ *Task, s string) Option[Bytes] {
	d, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return None[Bytes]()
	}
	return Some(Bytes(d))
}

// Time_from_unix_millis is `Time.from_unix_millis(ms)`.
func Time_from_unix_millis(_ *Task, ms int64) Time { return Time(MulS(ms, 1e6, "")) }

// Atomic_max is `a.max(n)`: stores the larger of the value and n and
// returns the new value.
func Atomic_max(_ *Task, a *Atomic[int64], n int64) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n > a.v {
		a.v = n
	}
	return a.v
}

// Range_to_list is `(a..b).to_list()`.
func Range_to_list(_ *Task, r Range[int64]) List[int64] {
	var xs []int64
	for i := range r.All() {
		xs = append(xs, i)
	}
	return ListOf(xs...)
}

// Json_Patch_apply_to is `p.apply_to(current)`: the new value, or current when the
// key was absent or null.
func Json_Patch_apply_to[T any](_ *Task, p Json_Patch[T], current T) T {
	if p.Tag == 3 {
		return p.P.(Json_Patch_Value[T]).F_v
	}
	return current
}

// Json_Patch_apply_opt is `p.apply_opt(current)` for optional fields: absent keeps
// current, null clears it, a value sets it.
func Json_Patch_apply_opt[T any](_ *Task, p Json_Patch[T], current Option[T]) Option[T] {
	switch p.Tag {
	case 2:
		return None[T]()
	case 3:
		return Some(p.P.(Json_Patch_Value[T]).F_v)
	}
	return current
}

// Bytes_find is `b.find(sub)`: bytes.Index.
func Bytes_find(_ *Task, b, sub Bytes) Option[int64] {
	i := bytes.Index([]byte(b), []byte(sub))
	if i < 0 {
		return Option[int64]{}
	}
	return Some(int64(i))
}

// Bytes_rfind is `b.rfind(sub)`: bytes.LastIndex.
func Bytes_rfind(_ *Task, b, sub Bytes) Option[int64] {
	i := bytes.LastIndex([]byte(b), []byte(sub))
	if i < 0 {
		return Option[int64]{}
	}
	return Some(int64(i))
}

// Bytes_zeroed is `Bytes.zeroed(n)`: make([]byte, n).
func Bytes_zeroed(_ *Task, n int64) Bytes {
	if n < 0 {
		FaultHere("negative length %d for Bytes.zeroed", n)
	}
	return Bytes(make([]byte, n))
}
