package rt

import "html"

// Html_escape_string is std/html's escape_string on the Go backend (decision
// 0597 part 2: declared bodyless in std/html.xo, the native backends run
// std/native/html_native.xo's): Go's html.EscapeString. unescape_string is
// generated from std/html.xo.
func Html_escape_string(_ *Task, s string) string { return html.EscapeString(s) }
