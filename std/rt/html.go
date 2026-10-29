package rt

import "html"

// std/html on the Go backend (decision 0408): Go's html package itself;
// std/native/html.xo ports it.

func Html_escape_string(_ *Task, s string) string   { return html.EscapeString(s) }
func Html_unescape_string(_ *Task, s string) string { return html.UnescapeString(s) }
