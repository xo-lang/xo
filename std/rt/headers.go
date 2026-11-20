package rt

// Header fields with repeated names (std.md 4.3, decision 0288): a
// Request's or Response's headers Map is the view; XFields holds every
// field ([name, value, ...]) when a name repeats. std/native/http.xo
// values_of and friends are the native backends'.

import (
	"net/textproto"
	"strings"
)

// fieldView is the Map's value of a name's fields: joined with ", ",
// except Set-Cookie's first (cookies are never joined).
func fieldView(k string, vs []string) string {
	if k == "Set-Cookie" {
		return vs[0]
	}
	return strings.Join(vs, ", ")
}

func appendFields(fs []string, k string, vs []string) []string {
	for _, v := range vs {
		fs = append(fs, textOf(k), textOf(v))
	}
	return fs
}

// valuesOf is a name's values: its fields while the Map still shows them,
// else the Map's one value (a direct change of headers wins), else none.
func valuesOf(h Map[string, string], fs []string, name string) []string {
	k := textproto.CanonicalMIMEHeaderKey(name)
	mv := headerGet(h, k)
	if !mv.Some {
		return nil
	}
	var vs []string
	for i := 0; i+1 < len(fs); i += 2 {
		if fs[i] == k {
			vs = append(vs, fs[i+1])
		}
	}
	if len(vs) > 0 && fieldView(k, vs) == mv.V {
		return vs
	}
	return []string{mv.V}
}

func withoutField(fs []string, k string) []string {
	if len(fs) == 0 {
		return fs
	}
	out := make([]string, 0, len(fs))
	for i := 0; i+1 < len(fs); i += 2 {
		if fs[i] != k {
			out = append(out, fs[i], fs[i+1])
		}
	}
	return out
}

// withValues sets name k's values to vs in a copy of h and fs.
func withValues(tk *Task, h Map[string, string], fs []string, k string, vs []string) (Map[string, string], []string) {
	Map_put(tk, &h, k, fieldView(k, vs))
	out := withoutField(fs, k)
	if len(vs) > 1 {
		for _, v := range vs {
			out = append(out, k, v)
		}
	}
	return h, out
}

// headerGet is a header map's value for name, matched as Go's
// CanonicalMIMEHeaderKey, then case-insensitively for a token name.
func headerGet(h Map[string, string], name string) Option[string] {
	if v := Map_get(nil, h, textproto.CanonicalMIMEHeaderKey(name)); v.Some {
		return v
	}
	if !isTokenName(name) {
		return Option[string]{}
	}
	for k, v := range Map_All(h) {
		if strings.EqualFold(k, name) {
			return Some(v)
		}
	}
	return Option[string]{}
}

// isTokenName is httpguts.ValidHeaderFieldName: one or more token bytes.
func isTokenName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '!' || c >= '#' && c <= '\'' || c == '*' || c == '+' || c == '-' ||
			c == '.' || c == '^' || c == '_' || c == '`' || c == '|' || c == '~'
		if !ok {
			return false
		}
	}
	return true
}
