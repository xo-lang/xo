package rt

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"math"
	"unicode/utf8"
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
	for {
		old := a.n.Load()
		if n <= old || a.n.CompareAndSwap(old, n) {
			return max(old, n)
		}
	}
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

// Bytes_contains is `b.contains(sub)`: bytes.Contains.
func Bytes_contains(_ *Task, b, sub Bytes) bool {
	return bytes.Contains([]byte(b), []byte(sub))
}

// Bytes_starts_with is `b.starts_with(p)`: bytes.HasPrefix.
func Bytes_starts_with(_ *Task, b, p Bytes) bool {
	return bytes.HasPrefix([]byte(b), []byte(p))
}

// Bytes_ends_with is `b.ends_with(p)`: bytes.HasSuffix.
func Bytes_ends_with(_ *Task, b, p Bytes) bool {
	return bytes.HasSuffix([]byte(b), []byte(p))
}

// Bytes_count is `b.count(sub)`: bytes.Count.
func Bytes_count(_ *Task, b, sub Bytes) int64 {
	return int64(bytes.Count([]byte(b), []byte(sub)))
}

// bytesList is parts as a List[Bytes], the Go backend's Bytes being a string.
func bytesList(parts [][]byte) List[Bytes] {
	out := make([]Bytes, len(parts))
	for i, p := range parts {
		out[i] = Bytes(p)
	}
	return ListOf(out...)
}

// Bytes_split is `b.split(sep)`: bytes.Split.
func Bytes_split(_ *Task, b, sep Bytes) List[Bytes] {
	return bytesList(bytes.Split([]byte(b), []byte(sep)))
}

// Bytes_split_n is `b.split_n(sep, n)`: bytes.SplitN.
func Bytes_split_n(_ *Task, b, sep Bytes, n int64) List[Bytes] {
	return bytesList(bytes.SplitN([]byte(b), []byte(sep), int(n)))
}

// Bytes_find_any is `b.find_any(chars)`: bytes.IndexAny, the set of runes
// chars being text, as Go's takes a string.
func Bytes_find_any(_ *Task, b Bytes, chars string) Option[int64] {
	i := bytes.IndexAny([]byte(b), chars)
	if i < 0 {
		return Option[int64]{}
	}
	return Some(int64(i))
}

// Bytes_rfind_any is `b.rfind_any(chars)`: bytes.LastIndexAny.
func Bytes_rfind_any(_ *Task, b Bytes, chars string) Option[int64] {
	i := bytes.LastIndexAny([]byte(b), chars)
	if i < 0 {
		return Option[int64]{}
	}
	return Some(int64(i))
}

// Bytes_contains_any is `b.contains_any(chars)`: bytes.ContainsAny.
func Bytes_contains_any(_ *Task, b Bytes, chars string) bool {
	return bytes.ContainsAny([]byte(b), chars)
}

// Bytes_contains_rune is `b.contains_rune(r)`: bytes.ContainsRune.
func Bytes_contains_rune(_ *Task, b Bytes, r Rune) bool {
	return bytes.ContainsRune([]byte(b), rune(r))
}

// Bytes_find_where is `b.find_where(f)`: the byte offset of the first rune
// for which f is true (bytes.IndexFunc, with the effect of f).
func Bytes_find_where[E any](tk *Task, b Bytes, f func(*Task, Rune) Result[bool, E]) Result[Option[int64], E] {
	for i, r := range string(b) {
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[Option[int64]](v.E, v.Tr)
		}
		if v.V {
			return Ok[Option[int64], E](Some(int64(i)))
		}
	}
	return Ok[Option[int64], E](None[int64]())
}

// Bytes_rfind_where is `b.rfind_where(f)`: the byte offset of the last rune
// for which f is true (bytes.LastIndexFunc).
func Bytes_rfind_where[E any](tk *Task, b Bytes, f func(*Task, Rune) Result[bool, E]) Result[Option[int64], E] {
	s := string(b)
	for i := len(s); i > 0; {
		r, size := utf8.DecodeLastRuneInString(s[0:i])
		i -= size
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[Option[int64]](v.E, v.Tr)
		}
		if v.V {
			return Ok[Option[int64], E](Some(int64(i)))
		}
	}
	return Ok[Option[int64], E](None[int64]())
}

// Bytes_contains_where is `b.contains_where(f)` (bytes.ContainsFunc).
func Bytes_contains_where[E any](tk *Task, b Bytes, f func(*Task, Rune) Result[bool, E]) Result[bool, E] {
	for _, r := range string(b) {
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[bool](v.E, v.Tr)
		}
		if v.V {
			return Ok[bool, E](true)
		}
	}
	return Ok[bool, E](false)
}

// Bytes_trim_start_where is `b.trim_start_where(f)` (bytes.TrimLeftFunc).
func Bytes_trim_start_where[E any](tk *Task, b Bytes, f func(*Task, Rune) Result[bool, E]) Result[Bytes, E] {
	s := string(b)
	for i, r := range s {
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[Bytes](v.E, v.Tr)
		}
		if !v.V {
			return Ok[Bytes, E](Bytes(s[i:]))
		}
	}
	return Ok[Bytes, E](Bytes(""))
}

// Bytes_trim_end_where is `b.trim_end_where(f)` (bytes.TrimRightFunc).
func Bytes_trim_end_where[E any](tk *Task, b Bytes, f func(*Task, Rune) Result[bool, E]) Result[Bytes, E] {
	s := string(b)
	for i := len(s); i > 0; {
		r, size := utf8.DecodeLastRuneInString(s[0:i])
		i -= size
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[Bytes](v.E, v.Tr)
		}
		if !v.V {
			return Ok[Bytes, E](Bytes(s[0 : i+size]))
		}
	}
	return Ok[Bytes, E](Bytes(""))
}

// Bytes_trim_where is `b.trim_where(f)`: bytes.TrimFunc.
func Bytes_trim_where[E any](tk *Task, b Bytes, f func(*Task, Rune) Result[bool, E]) Result[Bytes, E] {
	a := Bytes_trim_start_where(tk, b, f)
	if !a.Ok {
		return a
	}
	return Bytes_trim_end_where(tk, a.V, f)
}

// Bytes_split_where is `b.split_where(f)`: the runs of runes for which f is
// false (bytes.FieldsFunc, one pass recording the spans, then the pieces).
func Bytes_split_where[E any](tk *Task, b Bytes, f func(*Task, Rune) Result[bool, E]) Result[List[Bytes], E] {
	s := string(b)
	spans := make([]strSpan, 0, 32)
	start := -1
	for end, r := range s {
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[List[Bytes]](v.E, v.Tr)
		}
		if v.V {
			if start >= 0 {
				spans = append(spans, strSpan{start, end})
				start = -1
			}
		} else if start < 0 {
			start = end
		}
	}
	if start >= 0 {
		spans = append(spans, strSpan{start, len(s)})
	}
	out := make([]Bytes, len(spans))
	for i, sp := range spans {
		out[i] = Bytes(s[sp.start:sp.end])
	}
	return Ok[List[Bytes], E](ListOf(out...))
}

// Bytes_map_runes is `b.map_runes(f)`: f applied to every rune, None
// dropping it (bytes.Map), and b itself when nothing changes.
func Bytes_map_runes[E any](tk *Task, b Bytes, f func(*Task, Rune) Result[Option[Rune], E]) Result[Bytes, E] {
	r := Str_map_runes(tk, string(b), f)
	if !r.Ok {
		return ErrTr[Bytes](r.E, r.Tr)
	}
	if r.V == string(b) {
		return Ok[Bytes, E](b)
	}
	return Ok[Bytes, E](Bytes(r.V))
}

// Bytes_cut is `b.cut(sep)`: the bytes before and after the first sep
// (bytes.Cut), and None when sep is not there.
func Bytes_cut(_ *Task, b, sep Bytes) Option[Tuple2[Bytes, Bytes]] {
	before, after, ok := bytes.Cut([]byte(b), []byte(sep))
	if !ok {
		return Option[Tuple2[Bytes, Bytes]]{}
	}
	return Some(Tuple2[Bytes, Bytes]{Bytes(before), Bytes(after)})
}

// Bytes_cut_prefix is `b.cut_prefix(p)`: the rest of b after a leading p
// (bytes.CutPrefix), and None when b does not start with it.
func Bytes_cut_prefix(_ *Task, b, p Bytes) Option[Bytes] {
	rest, ok := bytes.CutPrefix([]byte(b), []byte(p))
	if !ok {
		return Option[Bytes]{}
	}
	return Some(Bytes(rest))
}

// Bytes_cut_suffix is `b.cut_suffix(p)`: b up to a trailing p
// (bytes.CutSuffix), and None when b does not end with it.
func Bytes_cut_suffix(_ *Task, b, p Bytes) Option[Bytes] {
	up, ok := bytes.CutSuffix([]byte(b), []byte(p))
	if !ok {
		return Option[Bytes]{}
	}
	return Some(Bytes(up))
}

// Bytes_split_after is `b.split_after(sep)`: bytes.SplitAfter.
func Bytes_split_after(_ *Task, b, sep Bytes) List[Bytes] {
	return bytesList(bytes.SplitAfter([]byte(b), []byte(sep)))
}

// Bytes_split_after_n is `b.split_after_n(sep, n)`: bytes.SplitAfterN.
func Bytes_split_after_n(_ *Task, b, sep Bytes, n int64) List[Bytes] {
	return bytesList(bytes.SplitAfterN([]byte(b), []byte(sep), int(n)))
}

// Bytes_fields is `b.fields()`: bytes.Fields.
func Bytes_fields(_ *Task, b Bytes) List[Bytes] {
	return bytesList(bytes.Fields([]byte(b)))
}

// Bytes_trim is `b.trim()`: bytes.TrimSpace.
func Bytes_trim(_ *Task, b Bytes) Bytes {
	return Bytes(bytes.TrimSpace([]byte(b)))
}

// Bytes_trim_prefix is `b.trim_prefix(p)`: bytes.TrimPrefix.
func Bytes_trim_prefix(_ *Task, b, p Bytes) Bytes {
	return Bytes(bytes.TrimPrefix([]byte(b), []byte(p)))
}

// Bytes_trim_suffix is `b.trim_suffix(p)`: bytes.TrimSuffix.
func Bytes_trim_suffix(_ *Task, b, p Bytes) Bytes {
	return Bytes(bytes.TrimSuffix([]byte(b), []byte(p)))
}

// Bytes_replace is `b.replace(old, new)`: bytes.ReplaceAll, the receiver
// itself when nothing changes (a Bytes is immutable, so Go's string value
// is already that; the shortcut also skips Go's copy).
func Bytes_replace(_ *Task, b, old, new Bytes) Bytes {
	if len(old) > 0 && !bytes.Contains([]byte(b), []byte(old)) {
		return b
	}
	return Bytes(bytes.ReplaceAll([]byte(b), []byte(old), []byte(new)))
}

// Bytes_replace_n is `b.replace_n(old, new, n)`: bytes.Replace, the
// receiver when nothing changes.
func Bytes_replace_n(_ *Task, b, old, new Bytes, n int64) Bytes {
	if string(old) == string(new) || n == 0 {
		return b
	}
	if len(old) > 0 && !bytes.Contains([]byte(b), []byte(old)) {
		return b
	}
	return Bytes(bytes.Replace([]byte(b), []byte(old), []byte(new), int(n)))
}

// Bytes_to_lower is `b.to_lower()`: bytes.ToLower (an invalid byte as
// U+FFFD, as Go maps it).
func Bytes_to_lower(_ *Task, b Bytes) Bytes { return Bytes(bytes.ToLower([]byte(b))) }

// Bytes_to_upper is `b.to_upper()`: bytes.ToUpper.
func Bytes_to_upper(_ *Task, b Bytes) Bytes { return Bytes(bytes.ToUpper([]byte(b))) }

// Bytes_equal_fold is `b.equal_fold(other)`: bytes.EqualFold.
func Bytes_equal_fold(_ *Task, b, other Bytes) bool {
	return bytes.EqualFold([]byte(b), []byte(other))
}

// Bytes_repeat is `b.repeat(n)`: bytes.Repeat, empty for n 0 and a fault
// for n below 0, as Go's and as Str_repeat (native raw.bytes_repeat).
func Bytes_repeat(_ *Task, b Bytes, n int64) Bytes {
	if n < 0 {
		FaultHere("negative repeat count %d", n)
	}
	if n == 0 || len(b) == 0 {
		return Bytes("")
	}
	if int64(len(b)) > math.MaxInt64/n {
		FaultHere("bytes: Repeat output length overflow")
	}
	if int64(len(b))*n > 4294967295 {
		FaultHere("bytes: Repeat output over 4 GiB")
	}
	return Bytes(bytes.Repeat([]byte(b), int(n)))
}

// Bytes_indexed is `b.indexed()`: each byte with its offset.
func Bytes_indexed(_ *Task, b Bytes) List[Tuple2[int64, uint8]] {
	out := make([]Tuple2[int64, uint8], len(b))
	for i := 0; i < len(b); i++ {
		out[i] = Tuple2[int64, uint8]{int64(i), b[i]}
	}
	return ListOf(out...)
}

// Bytes_zeroed is `Bytes.zeroed(n)`: make([]byte, n).
func Bytes_zeroed(_ *Task, n int64) Bytes {
	if n < 0 {
		FaultHere("negative length %d for Bytes.zeroed", n)
	}
	return Bytes(make([]byte, n))
}

// Str_clone is `s.clone()`: Go strings are immutable and substrings share by
// design, so the copy is the value itself (decision 0330 views are native only).
func Str_clone(_ *Task, s string) string { return s }

// Bytes_clone is `b.clone()`: Bytes is a Go string here, so it is its own copy.
func Bytes_clone(_ *Task, b Bytes) Bytes { return b }
