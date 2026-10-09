package mod

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
)

// Deny is one import direction rule of an xo.mod (decision 0095):
//
//	deny example.com/notes/domain -> example.com/notes/store, example.com/notes/api
//
// A package matching From may not `use` a package matching any of To.
// A pattern is a package path, or a path ending in `/...` that matches the
// path and every package under it.
type Deny struct {
	From string
	To   []string
	Line int // 1 based line in xo.mod
}

// String is the rule in canonical form, as `xo fmt` would write it.
func (d Deny) String() string {
	return "deny " + d.From + " -> " + strings.Join(d.To, ", ")
}

// Match reports whether package path p matches pattern pat.
func Match(pat, p string) bool {
	if base, ok := strings.CutSuffix(pat, "/..."); ok {
		return p == base || strings.HasPrefix(p, base+"/")
	}
	return p == pat
}

// Denies reports the first target pattern of the rule that forbids an
// import of to by from, or "".
func (d Deny) Denies(from, to string) string {
	if !Match(d.From, from) {
		return ""
	}
	for _, t := range d.To {
		if Match(t, to) {
			return t
		}
	}
	return ""
}

// PatternBase is a pattern without its `/...` suffix.
func PatternBase(pat string) string {
	base, _ := strings.CutSuffix(pat, "/...")
	return base
}

// validPattern reports whether pat is a package path or a path with a
// trailing `/...`: slash separated non empty elements, none `.` or `..`,
// and no `...` elsewhere.
func validPattern(pat string) bool {
	base := PatternBase(pat)
	if base == "" || strings.HasPrefix(base, "/") || strings.HasSuffix(base, "/") {
		return false
	}
	for _, e := range strings.Split(base, "/") {
		if e == "" || e == "." || e == ".." || strings.Contains(e, "...") || strings.ContainsAny(e, ",> \t") {
			return false
		}
	}
	return true
}

// parseDeny parses the text of a `deny` line after the keyword.
func parseDeny(name string, line int, rest string) (Deny, *Problem) {
	bad := func(msg string) (Deny, *Problem) {
		return Deny{}, &Problem{Code: diag.ModFileInvalid, File: name, Line: line, Msg: msg,
			Notes: []string{"an import rule is `deny <package> -> <package>, ...`; a path ending in `/...` matches every package under it (decision 0095)"}}
	}
	from, to, ok := strings.Cut(rest, "->")
	if !ok {
		return bad("malformed deny: want `deny <package> -> <package>, ...`")
	}
	d := Deny{From: strings.TrimSpace(from), Line: line}
	if !validPattern(d.From) {
		return bad(fmt.Sprintf("malformed deny: invalid package pattern %q before `->`", d.From))
	}
	for _, t := range strings.Split(to, ",") {
		t = strings.TrimSpace(t)
		if !validPattern(t) {
			return bad(fmt.Sprintf("malformed deny: invalid package pattern %q after `->`", t))
		}
		d.To = append(d.To, t)
	}
	return d, nil
}
