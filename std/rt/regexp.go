package rt

import "unicode"

// std/regexp on the Go backend (decision 0597 part 2): the port in
// std/regexp.xo, generated into this package (std/rt_hosted.txt), with
// these twins of its two bodyless functions (the native backends run
// std/native/regexp_native.xo's).

// Regexp_must is `regexp.must(pattern)`: compile, faulting at the caller.
func Regexp_must(tk *Task, pattern string) Regexp_Regex {
	r := Regexp_compile(tk, pattern)
	if !r.Ok {
		FaultHere("%s", r.E.M_display(tk))
	}
	return r.V
}

// Regexp_simple_fold is Go's unicode.SimpleFold.
func Regexp_simple_fold(_ *Task, r int64) int64 {
	if r < 0 || r > unicode.MaxRune {
		return r
	}
	return int64(unicode.SimpleFold(rune(r)))
}

// Regexp_zeros is `zeros(n)`: n zeros in one allocation.
func Regexp_zeros(_ *Task, n int64) List[int64] {
	return ListOf(make([]int64, n)...)
}
