package rt

// M_display is the display method of std/path's PathErr (std.md 9): Go's
// ErrBadPattern text with the pattern, quoted as Str.quote quotes. path's
// functions are std/native/path.xo on the Go backend too (decision 0312);
// this, like the other display methods here, is hand written Go.

func (e Path_PathErr) M_display(t *Task) string {
	return "invalid path pattern " + Str_quote(t, e.F_pattern) + ": syntax error in pattern"
}
