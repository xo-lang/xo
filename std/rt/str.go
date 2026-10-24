package rt

import (
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Bytes is the immutable byte sequence `Bytes`.
type Bytes string

// Rune is a Unicode scalar value. It is a distinct type from Int32 so that
// it displays as a character.
type Rune int32

// StrSlice is `s[lo..hi]` on a Str: byte offsets that must fall on UTF-8
// boundaries (core 3.6).
func StrSlice(s string, lo, hi int64, hasLo, hasHi bool, at string) string {
	n := int64(len(s))
	if !hasLo {
		lo = 0
	}
	if !hasHi {
		hi = n
	}
	if lo < 0 || hi > n || lo > hi {
		Faultf(at, "slice %d..%d out of range for Str of length %d", lo, hi, n)
	}
	if (lo < n && !utf8.RuneStart(s[lo])) || (hi < n && !utf8.RuneStart(s[hi])) {
		Faultf(at, "slice %d..%d of Str splits a UTF-8 sequence", lo, hi)
	}
	return s[lo:hi]
}

// BytesSlice is `b[lo..hi]` on Bytes.
func BytesSlice(b Bytes, lo, hi int64, hasLo, hasHi bool, at string) Bytes {
	n := int64(len(b))
	if !hasLo {
		lo = 0
	}
	if !hasHi {
		hi = n
	}
	if lo < 0 || hi > n || lo > hi {
		Faultf(at, "slice %d..%d out of range for Bytes of length %d", lo, hi, n)
	}
	return b[lo:hi]
}

// BytesAt is `b[i]`. The fault is an inline panic (as List_AtM's), so
// BytesAt inlines into the generated code: with Faultf's call it cost 81
// against Go's budget of 80, a call for every byte read (2026-10-21,
// std/path's clean spent 15% of its time in it).
func BytesAt(b Bytes, i int64, at string) uint8 {
	if uint64(i) >= uint64(len(b)) {
		panic(&arithFault{i, int64(len(b)), 'b', at})
	}
	return b[i]
}

// Str_len is `s.len()` in bytes.
func Str_len(_ *Task, s string) int64 { return int64(len(s)) }

// Str_rune_count is `s.rune_count()`.
func Str_rune_count(_ *Task, s string) int64 { return int64(utf8.RuneCountInString(s)) }

// Str_is_empty is `s.is_empty()`.
func Str_is_empty(_ *Task, s string) bool { return s == "" }

// Str_contains is `s.contains(sub)`.
func Str_contains(_ *Task, s, sub string) bool { return strings.Contains(s, sub) }

// Str_starts_with is `s.starts_with(p)`.
func Str_starts_with(_ *Task, s, p string) bool { return strings.HasPrefix(s, p) }

// Str_ends_with is `s.ends_with(p)`.
func Str_ends_with(_ *Task, s, p string) bool { return strings.HasSuffix(s, p) }

// Str_equal_fold is `s.equal_fold(other)`: equality under Unicode simple
// case folding (strings.EqualFold), not only ASCII case.
func Str_equal_fold(_ *Task, a, b string) bool { return strings.EqualFold(a, b) }

// Str_find is `s.find(sub)`: a byte offset.
func Str_find(_ *Task, s, sub string) Option[int64] {
	i := strings.Index(s, sub)
	if i < 0 {
		return Option[int64]{}
	}
	return Some(int64(i))
}

// Str_count is `s.count(sub)`: the number of non-overlapping copies of sub
// (strings.Count), the rune count plus one for "".
func Str_count(_ *Task, s, sub string) int64 { return int64(strings.Count(s, sub)) }

// Str_find_any is `s.find_any(chars)`: the byte offset of the first rune of
// chars in s (strings.IndexAny).
func Str_find_any(_ *Task, s, chars string) Option[int64] {
	i := strings.IndexAny(s, chars)
	if i < 0 {
		return Option[int64]{}
	}
	return Some(int64(i))
}

// Str_rfind_any is `s.rfind_any(chars)`: the byte offset of the last rune of
// chars in s (strings.LastIndexAny).
func Str_rfind_any(_ *Task, s, chars string) Option[int64] {
	i := strings.LastIndexAny(s, chars)
	if i < 0 {
		return Option[int64]{}
	}
	return Some(int64(i))
}

// Str_contains_any is `s.contains_any(chars)`: strings.ContainsAny.
func Str_contains_any(_ *Task, s, chars string) bool { return strings.ContainsAny(s, chars) }

// Str_contains_rune is `s.contains_rune(r)`: strings.ContainsRune.
func Str_contains_rune(_ *Task, s string, r Rune) bool { return strings.ContainsRune(s, rune(r)) }

// Str_cut is `s.cut(sep)`: the text before and after the first sep
// (strings.Cut), and None when sep is not there.
func Str_cut(_ *Task, s, sep string) Option[Tuple2[string, string]] {
	before, after, ok := strings.Cut(s, sep)
	if !ok {
		return None[Tuple2[string, string]]()
	}
	return Some(Tuple2[string, string]{before, after})
}

// Str_cut_prefix is `s.cut_prefix(prefix)`: the text after a leading
// prefix (strings.CutPrefix), and None when s does not start with it.
func Str_cut_prefix(_ *Task, s, prefix string) Option[string] {
	rest, ok := strings.CutPrefix(s, prefix)
	if !ok {
		return Option[string]{}
	}
	return Some(rest)
}

// Str_cut_suffix is `s.cut_suffix(suffix)`: the text before a trailing
// suffix (strings.CutSuffix), and None when s does not end with it.
func Str_cut_suffix(_ *Task, s, suffix string) Option[string] {
	before, ok := strings.CutSuffix(s, suffix)
	if !ok {
		return Option[string]{}
	}
	return Some(before)
}

// Str_split is `s.split(sep)`.
func Str_split(_ *Task, s, sep string) List[string] { return ListOf(strings.Split(s, sep)...) }

// Str_split_n is `s.split_n(sep, n)`: at most n pieces, the last holding the
// rest, n below 0 for all and 0 for none (strings.SplitN).
func Str_split_n(_ *Task, s, sep string, n int64) List[string] {
	return ListOf(strings.SplitN(s, sep, int(n))...)
}

// Str_split_after is `s.split_after(sep)`: like split, each piece keeping
// its separator (strings.SplitAfter).
func Str_split_after(_ *Task, s, sep string) List[string] {
	return ListOf(strings.SplitAfter(s, sep)...)
}

// Str_split_after_n is `s.split_after_n(sep, n)` (strings.SplitAfterN).
func Str_split_after_n(_ *Task, s, sep string, n int64) List[string] {
	return ListOf(strings.SplitAfterN(s, sep, int(n))...)
}

// Str_fields is `s.fields()`: the runs of non-white-space text
// (strings.Fields, unicode.IsSpace).
func Str_fields(_ *Task, s string) List[string] { return ListOf(strings.Fields(s)...) }

// Str_lines is `s.lines()`: splits on \n and strips a trailing \r. A final
// newline does not produce an empty last line.
func Str_lines(_ *Task, s string) List[string] {
	if s == "" {
		return List[string]{}
	}
	parts := strings.Split(s, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	for i, p := range parts {
		parts[i] = strings.TrimSuffix(p, "\r")
	}
	return ListOf(parts...)
}

// Str_trim is `s.trim()`.
func Str_trim(_ *Task, s string) string { return strings.TrimSpace(s) }

// Str_trim_start is `s.trim_start()`.
func Str_trim_start(_ *Task, s string) string { return strings.TrimLeftFunc(s, unicode.IsSpace) }

// Str_trim_end is `s.trim_end()`.
func Str_trim_end(_ *Task, s string) string { return strings.TrimRightFunc(s, unicode.IsSpace) }

// Str_trim_prefix is `s.trim_prefix(p)`.
func Str_trim_prefix(_ *Task, s, p string) string { return strings.TrimPrefix(s, p) }

// Str_trim_chars is `s.trim_chars(chars)`: strings.Trim.
func Str_trim_chars(_ *Task, s, chars string) string { return strings.Trim(s, chars) }

// Str_trim_start_chars is `s.trim_start_chars(chars)`: strings.TrimLeft.
func Str_trim_start_chars(_ *Task, s, chars string) string { return strings.TrimLeft(s, chars) }

// Str_trim_end_chars is `s.trim_end_chars(chars)`: strings.TrimRight.
func Str_trim_end_chars(_ *Task, s, chars string) string { return strings.TrimRight(s, chars) }

// Str_trim_suffix is `s.trim_suffix(p)`.
func Str_trim_suffix(_ *Task, s, p string) string { return strings.TrimSuffix(s, p) }

// Str_to_lower is `s.to_lower()`.
func Str_to_lower(_ *Task, s string) string { return strings.ToLower(s) }

// Str_eq_lower is `a.to_lower() == b.to_lower()` without building the
// lowered strings (generated for that expression). While both strings
// are ASCII it compares lowered bytes; at the first non-ASCII byte it
// falls back to ToLower on both, so the result is always the same.
// A mismatch in the ASCII prefix is final: ASCII bytes lower to one byte
// each at the same position whatever follows.
func Str_eq_lower(a, b string) bool {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		ca, cb := a[i], b[i]
		if ca >= 0x80 || cb >= 0x80 {
			return strings.ToLower(a[i:]) == strings.ToLower(b[i:])
		}
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	if len(a) == len(b) {
		return true
	}
	// one is a prefix of the other: equal only if the rest lowers to
	// nothing, which no non-empty string does
	return false
}

// Str_to_upper is `s.to_upper()`.
func Str_to_upper(_ *Task, s string) string { return strings.ToUpper(s) }

// Str_replace is `s.replace(old, new)`: every occurrence.
func Str_replace(_ *Task, s, old, nw string) string { return strings.ReplaceAll(s, old, nw) }

// Str_replace_n is `s.replace_n(old, new, n)`: the first n copies of old
// replaced (n below 0 for every one, 0 for none; strings.Replace).
func Str_replace_n(_ *Task, s, old, new string, n int64) string {
	return strings.Replace(s, old, new, int(n))
}

// Str_repeat is `s.repeat(n)`.
func Str_repeat(_ *Task, s string, n int64) string {
	if n < 0 {
		FaultHere("negative repeat count %d", n)
	}
	if n == 0 || len(s) == 0 {
		return ""
	}
	if int64(len(s)) > math.MaxInt64/n {
		FaultHere("strings: Repeat output length overflow")
	}
	if int64(len(s))*n > 4294967295 {
		FaultHere("strings: Repeat output over 4 GiB")
	}
	return strings.Repeat(s, int(n))
}

// Str_runes is `s.runes()`.
func Str_runes(_ *Task, s string) List[Rune] {
	out := make([]Rune, 0, len(s))
	for _, r := range s {
		out = append(out, Rune(r))
	}
	return ListOf(out...)
}

// Str_find_where is `s.find_where(f)`: the byte offset of the first rune
// for which f is true (strings.IndexFunc, with the effect of f).
func Str_find_where[E any](tk *Task, s string, f func(*Task, Rune) Result[bool, E]) Result[Option[int64], E] {
	for i, r := range s {
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

// Str_rfind_where is `s.rfind_where(f)`: the byte offset of the last rune
// for which f is true (strings.LastIndexFunc).
func Str_rfind_where[E any](tk *Task, s string, f func(*Task, Rune) Result[bool, E]) Result[Option[int64], E] {
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

// Str_contains_where is `s.contains_where(f)` (strings.ContainsFunc).
func Str_contains_where[E any](tk *Task, s string, f func(*Task, Rune) Result[bool, E]) Result[bool, E] {
	for _, r := range s {
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

// Str_trim_start_where is `s.trim_start_where(f)` (strings.TrimLeftFunc).
func Str_trim_start_where[E any](tk *Task, s string, f func(*Task, Rune) Result[bool, E]) Result[string, E] {
	for i, r := range s {
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[string](v.E, v.Tr)
		}
		if !v.V {
			return Ok[string, E](s[i:])
		}
	}
	return Ok[string, E]("")
}

// Str_trim_end_where is `s.trim_end_where(f)` (strings.TrimRightFunc).
func Str_trim_end_where[E any](tk *Task, s string, f func(*Task, Rune) Result[bool, E]) Result[string, E] {
	for i := len(s); i > 0; {
		r, size := utf8.DecodeLastRuneInString(s[0:i])
		i -= size
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[string](v.E, v.Tr)
		}
		if !v.V {
			return Ok[string, E](s[0 : i+size])
		}
	}
	return Ok[string, E]("")
}

// Str_trim_where is `s.trim_where(f)`: strings.TrimFunc.
func Str_trim_where[E any](tk *Task, s string, f func(*Task, Rune) Result[bool, E]) Result[string, E] {
	a := Str_trim_start_where(tk, s, f)
	if !a.Ok {
		return a
	}
	return Str_trim_end_where(tk, a.V, f)
}

// strSpan is a run of s that Str_split_where keeps: s[start:end].
type strSpan struct{ start, end int }

// Str_split_where is `s.split_where(f)`: the runs of runes for which f is
// false (strings.FieldsFunc, one pass recording the spans, then the
// pieces).
func Str_split_where[E any](tk *Task, s string, f func(*Task, Rune) Result[bool, E]) Result[List[string], E] {
	spans := make([]strSpan, 0, 32)
	start := -1
	for end, r := range s {
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[List[string]](v.E, v.Tr)
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
	out := make([]string, len(spans))
	for i, sp := range spans {
		out[i] = s[sp.start:sp.end]
	}
	return Ok[List[string], E](ListOf(out...))
}

// Str_map_runes is `s.map_runes(f)`: f applied to every rune, None
// dropping it (strings.Map, with the option in place of Go's negative
// rune). No copy until the first rune that changes; a malformed byte
// counts as a change, a valid U+FFFD that maps to itself does not.
func Str_map_runes[E any](tk *Task, s string, f func(*Task, Rune) Result[Option[Rune], E]) Result[string, E] {
	var first Option[Rune]
	firstSize := 0
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[string](v.E, v.Tr)
		}
		if !v.V.Some || v.V.V != Rune(r) || (r == utf8.RuneError && size == 1) {
			first, firstSize = v.V, size
			break
		}
		i += size
	}
	if i >= len(s) {
		return Ok[string, E](s)
	}
	var b strings.Builder
	b.Grow(len(s) + utf8.UTFMax)
	b.WriteString(s[:i])
	if first.Some {
		b.WriteRune(rune(first.V))
	}
	i += firstSize
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		v := f(tk, Rune(r))
		if !v.Ok {
			return ErrTr[string](v.E, v.Tr)
		}
		if v.V.Some {
			b.WriteRune(rune(v.V.V))
		}
		i += size
	}
	return Ok[string, E](b.String())
}

// Str_bytes is `s.bytes()`.
func Str_bytes(_ *Task, s string) Bytes { return Bytes(s) }

// Bytes_len is `b.len()`.
func Bytes_len(_ *Task, b Bytes) int64 { return int64(len(b)) }

// Bytes_is_empty is `b.is_empty()`.
func Bytes_is_empty(_ *Task, b Bytes) bool { return len(b) == 0 }

// Bytes_to_str is `b.to_str()`: None if b is not valid UTF-8.
func Bytes_to_str(_ *Task, b Bytes) Option[string] {
	if !utf8.ValidString(string(b)) {
		return Option[string]{}
	}
	return Some(string(b))
}

// Rune_is_digit is `r.is_digit()`.
func Rune_is_digit(_ *Task, r Rune) bool { return unicode.IsDigit(rune(r)) }

// Rune_is_letter is `r.is_letter()`.
func Rune_is_letter(_ *Task, r Rune) bool { return unicode.IsLetter(rune(r)) }

// Rune_is_space is `r.is_space()`.
func Rune_is_space(_ *Task, r Rune) bool { return unicode.IsSpace(rune(r)) }

// Rune_is_upper is `r.is_upper()`.
func Rune_is_upper(_ *Task, r Rune) bool { return unicode.IsUpper(rune(r)) }

// Rune_is_lower is `r.is_lower()`.
func Rune_is_lower(_ *Task, r Rune) bool { return unicode.IsLower(rune(r)) }

// Rune_is_title is `r.is_title()`.
func Rune_is_title(_ *Task, r Rune) bool { return unicode.IsTitle(rune(r)) }

// Rune_is_punct is `r.is_punct()`.
func Rune_is_punct(_ *Task, r Rune) bool { return unicode.IsPunct(rune(r)) }

// Rune_is_control is `r.is_control()`.
func Rune_is_control(_ *Task, r Rune) bool { return unicode.IsControl(rune(r)) }

// Rune_is_graphic is `r.is_graphic()`.
func Rune_is_graphic(_ *Task, r Rune) bool { return unicode.IsGraphic(rune(r)) }

// Rune_is_mark is `r.is_mark()`.
func Rune_is_mark(_ *Task, r Rune) bool { return unicode.IsMark(rune(r)) }

// Rune_is_symbol is `r.is_symbol()`.
func Rune_is_symbol(_ *Task, r Rune) bool { return unicode.IsSymbol(rune(r)) }

// Rune_is_number is `r.is_number()`.
func Rune_is_number(_ *Task, r Rune) bool { return unicode.IsNumber(rune(r)) }

// Rune_to_title is `r.to_title()`.
func Rune_to_title(_ *Task, r Rune) Rune { return Rune(unicode.ToTitle(rune(r))) }

// Rune_utf8_len is `r.utf8_len()`: utf8.RuneLen of a scalar value.
func Rune_utf8_len(_ *Task, r Rune) int64 { return int64(utf8.RuneLen(rune(r))) }

// Rune_to_lower is `r.to_lower()`.
func Rune_to_lower(_ *Task, r Rune) Rune { return Rune(unicode.ToLower(rune(r))) }

// Rune_to_upper is `r.to_upper()`.
func Rune_to_upper(_ *Task, r Rune) Rune { return Rune(unicode.ToUpper(rune(r))) }

// Interp concatenates the parts of an interpolated string.
func Interp(parts ...string) string { return strings.Join(parts, "") }

// Rune_is_print is `r.is_print()`: Go's strconv.IsPrint (decision 0079).
func Rune_is_print(_ *Task, r Rune) bool { return strconv.IsPrint(rune(r)) }

// Rune_quote is `r.quote()`: Go's strconv.QuoteRune.
func Rune_quote(_ *Task, r Rune) string { return strconv.QuoteRune(rune(r)) }

// Str_quote is `s.quote()`: Go's strconv.Quote.
func Str_quote(_ *Task, s string) string { return strconv.Quote(s) }

// Bytes_quote is `b.quote()`: Go's strconv.Quote of the bytes.
func Bytes_quote(_ *Task, b Bytes) string { return strconv.Quote(string(b)) }

// Bytes_rune_count is `b.rune_count()`: utf8.RuneCount, an invalid byte
// counting as one rune.
func Bytes_rune_count(_ *Task, b Bytes) int64 { return int64(utf8.RuneCountInString(string(b))) }

// Bytes_decode_rune is `b.decode_rune(i)`: utf8.DecodeRune of b[i..],
// (U+FFFD, 1) for an invalid or short sequence and (U+FFFD, 0) at the end.
// It faults when i is outside 0 to len.
func Bytes_decode_rune(_ *Task, b Bytes, i int64) Tuple2[Rune, int64] {
	if i < 0 || i > int64(len(b)) {
		FaultHere("index %d out of range for decode_rune on Bytes of length %d", i, len(b))
	}
	r, n := utf8.DecodeRuneInString(string(b[i:]))
	return Tuple2[Rune, int64]{F0: Rune(r), F1: int64(n)}
}

// Str_rfind is `s.rfind(sub)`: strings.LastIndex's byte offset.
func Str_rfind(_ *Task, s, sub string) Option[int64] {
	i := strings.LastIndex(s, sub)
	if i < 0 {
		return Option[int64]{}
	}
	return Some(int64(i))
}
