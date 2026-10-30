// Package stdindex makes spec/std.md's prelude index: every function and
// method std/prelude.xo declares, by type, as the prelude declares it
// (2026-10-20 poll: agents grepped the prelude for Str.find and
// Rand.bytes). scripts/stdsync writes it into spec/std.md beside the other
// files a prelude change derives (scripts/stdindex writes this one alone);
// the repotest TestStdIndexCurrent fails when the two disagree.
package stdindex

import (
	"fmt"
	"regexp"
	"strings"
)

// Begin and End mark the generated part of spec/std.md.
const (
	Begin = "<!-- BEGIN prelude index: generated from std/prelude.xo by `go run ./scripts/stdsync`; do not edit -->"
	End   = "<!-- END prelude index -->"
)

// beginPrefix finds the block in a spec whose Begin line still names an
// older command (the marker named scripts/stdindex until 2026-10-24):
// matching the prefix, not the whole line, means a rewording replaces the
// block instead of appending a second one.
const beginPrefix = "<!-- BEGIN prelude index"

var (
	ifaceRe  = regexp.MustCompile(`^interface ([A-Za-z0-9_]+)`)
	methodRe = regexp.MustCompile(`^fn \((?:var )?[a-z_]+: ([A-Za-z0-9_]+)`)
	freeRe   = regexp.MustCompile(`^fn [a-z_]`)
)

// Index is the markdown of the prelude index of prelude (std/prelude.xo's
// text): a section per type in the order the prelude first declares a
// method of it, its methods in source order, then the free functions.
func Index(prelude string) string {
	var order []string
	byType := map[string][]string{}
	var free []string
	add := func(t, sig string) {
		if _, ok := byType[t]; !ok {
			order = append(order, t)
		}
		byType[t] = append(byType[t], sig)
	}
	iface := ""
	for _, l := range strings.Split(prelude, "\n") {
		if m := ifaceRe.FindStringSubmatch(l); m != nil {
			// `interface Eq {}` has no methods and no closing line
			if !strings.HasSuffix(strings.TrimSpace(l), "{}") {
				iface = m[1]
			}
			continue
		}
		if iface != "" {
			if l == "}" {
				iface = ""
			} else if t := strings.TrimSpace(l); strings.HasPrefix(t, "fn ") {
				add(iface, t)
			}
			continue
		}
		if m := methodRe.FindStringSubmatch(l); m != nil {
			add(m[1], strings.TrimSpace(l))
		} else if freeRe.MatchString(l) {
			free = append(free, strings.TrimSpace(l))
		}
	}
	var b strings.Builder
	b.WriteString(Begin + "\n\n")
	b.WriteString("## Appendix A. Prelude index\n\n")
	b.WriteString("Every function and method of std/prelude.xo, by type, as declared there\n")
	b.WriteString("(the sections above explain them); interface methods take `self`. A\n")
	b.WriteString("`static` receiver is a function on the type (`Int.parse(s)`); one with an\n")
	b.WriteString("UPPER_SNAKE name and no parameters is a constant, read without a call\n")
	b.WriteString("(`Time.UNIX_EPOCH`).\n")
	n := len(free)
	for _, t := range order {
		n += len(byType[t])
	}
	fmt.Fprintf(&b, "%d declarations.\n", n)
	for _, t := range order {
		fmt.Fprintf(&b, "\n### %s\n\n```\n%s\n```\n", t, strings.Join(byType[t], "\n"))
	}
	if len(free) > 0 {
		fmt.Fprintf(&b, "\n### Functions\n\n```\n%s\n```\n", strings.Join(free, "\n"))
	}
	b.WriteString("\n" + End + "\n")
	return b.String()
}

// Splice is spec (spec/std.md's text) with its prelude index replaced by
// index, or appended when it has none.
func Splice(spec, index string) string {
	i := strings.Index(spec, beginPrefix)
	j := strings.Index(spec, End)
	if i < 0 || j < i {
		return strings.TrimRight(spec, "\n") + "\n\n" + index
	}
	rest := spec[j+len(End):]
	rest = strings.TrimPrefix(rest, "\n")
	return spec[:i] + index + rest
}

// Current is the index spec holds, "" for none.
func Current(spec string) string {
	i := strings.Index(spec, beginPrefix)
	j := strings.Index(spec, End)
	if i < 0 || j < i {
		return ""
	}
	return spec[i:j+len(End)] + "\n"
}
