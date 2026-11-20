//go:build !xo_nohttp

package rt

import (
	"strconv"
	"strings"
)

// Go's pattern conflicts (net/http pattern.go conflictsWith, task #238),
// as std/native/http.xo check_conflicts: two routes conflict when they
// match the same requests, or some requests each with neither more
// specific, and ServeMux.Handle refuses them. Relations: 0 equivalent,
// 1 more general, 2 more specific, 3 disjoint, 4 overlaps.

func segMulti(s routeSeg) bool { return s.rest }

func segWild(s routeSeg) bool { return !s.rest && !s.dollar && s.param != "" }

func segS(s routeSeg) string {
	switch {
	case s.rest || segWild(s):
		return s.param
	case s.dollar:
		return "/"
	}
	return s.lit
}

func inverseRel(r int) int {
	switch r {
	case 2:
		return 1
	case 1:
		return 2
	}
	return r
}

func combineRel(r1, r2 int) int {
	switch r1 {
	case 0:
		return r2
	case 3:
		return 3
	case 4:
		if r2 == 3 {
			return 3
		}
		return 4
	}
	if r2 == 0 {
		return r1
	}
	if r2 == inverseRel(r1) {
		return 4
	}
	return r2
}

func compareMethods(m1, m2 string) int {
	switch {
	case m1 == m2:
		return 0
	case m1 == "":
		return 1
	case m2 == "":
		return 2
	case m1 == "GET" && m2 == "HEAD":
		return 1
	case m2 == "GET" && m1 == "HEAD":
		return 2
	}
	return 3
}

func compareSegs(s1, s2 routeSeg) int {
	switch {
	case segMulti(s1) && segMulti(s2):
		return 0
	case segMulti(s1):
		return 1
	case segMulti(s2):
		return 2
	case segWild(s1) && segWild(s2):
		return 0
	case segWild(s1):
		if segS(s2) == "/" {
			return 3
		}
		return 1
	case segWild(s2):
		if segS(s1) == "/" {
			return 3
		}
		return 2
	}
	if segS(s1) == segS(s2) {
		return 0
	}
	return 3
}

func lastMulti(segs []routeSeg) bool { return len(segs) > 0 && segs[len(segs)-1].rest }

func comparePaths(a, b []routeSeg) int {
	if len(a) != len(b) && !lastMulti(a) && !lastMulti(b) {
		return 3
	}
	rel := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if rel = combineRel(rel, compareSegs(a[i], b[i])); rel == 3 {
			return rel
		}
	}
	switch {
	case len(a) == len(b):
		return rel
	case len(a) < len(b) && lastMulti(a):
		return combineRel(rel, 1)
	case len(b) < len(a) && lastMulti(b):
		return combineRel(rel, 2)
	}
	return 3
}

// routeText is the route's pattern as written (the method upper case).
func routeText(r *route) string {
	var t strings.Builder
	if r.method != "" {
		t.WriteString(r.method + " ")
	}
	for _, s := range r.segs {
		t.WriteByte('/')
		switch {
		case s.rest:
			if s.param != "" {
				t.WriteString("{" + s.param + "...}")
			}
		case s.dollar:
			t.WriteString("{$}")
		case s.param != "":
			if s.isInt {
				t.WriteString("{" + s.param + ":Int}")
			} else {
				t.WriteString("{" + s.param + "}")
			}
		default:
			t.WriteString(s.lit)
		}
	}
	return t.String()
}

func writeSeg(s routeSeg) string {
	if !segMulti(s) && segS(s) != "/" {
		return "/" + segS(s)
	}
	return "/"
}

func writeRest(segs []routeSeg, from int) string {
	t := ""
	for i := from; i < len(segs); i++ {
		t += writeSeg(segs[i])
	}
	return t
}

func commonPath(a, b []routeSeg) string {
	t := ""
	i := 0
	for ; i < len(a) && i < len(b); i++ {
		if segWild(a[i]) {
			t += writeSeg(b[i])
		} else {
			t += writeSeg(a[i])
		}
	}
	if i < len(a) {
		return t + writeRest(a, i)
	}
	return t + writeRest(b, i)
}

func differencePath(a, b []routeSeg) string {
	t := ""
	i := 0
	for ; i < len(a) && i < len(b); i++ {
		s1, s2 := a[i], b[i]
		if segMulti(s1) && segMulti(s2) {
			return t + "/"
		}
		if segMulti(s1) {
			t += "/"
			if segS(s2) == "/" {
				if segS(s1) == "" {
					t += "x"
				} else {
					t += segS(s1)
				}
			}
			return t
		}
		if segWild(s1) && !segWild(s2) && !segMulti(s2) && segS(s1) == segS(s2) {
			t += "/" + segS(s2) + "x"
		} else {
			t += writeSeg(s1)
		}
	}
	if i < len(a) {
		return t + writeRest(a, i)
	}
	return t + writeRest(b, i)
}

// conflictError is the reason nr conflicts with a route of routes, or "".
func conflictError(routes []*route, nr *route) string {
	for _, old := range routes {
		mrel := compareMethods(nr.method, old.method)
		if mrel == 3 {
			continue
		}
		prel := comparePaths(nr.segs, old.segs)
		rel := combineRel(mrel, prel)
		if rel != 0 && rel != 4 {
			continue
		}
		p1, p2 := routeText(nr), routeText(old)
		var why string
		switch {
		case rel == 0:
			why = p1 + " matches the same requests as " + p2
		case prel == 4:
			why = p1 + " and " + p2 + " both match some paths, like " + strconv.Quote(commonPath(nr.segs, old.segs)) + ".\nBut neither is more specific than the other.\n" +
				p1 + " matches " + strconv.Quote(differencePath(nr.segs, old.segs)) + ", but " + p2 + " doesn't.\n" +
				p2 + " matches " + strconv.Quote(differencePath(old.segs, nr.segs)) + ", but " + p1 + " doesn't."
		case mrel == 1 && prel == 2:
			why = p1 + " matches more methods than " + p2 + ", but has a more specific path pattern"
		default:
			why = p1 + " matches fewer methods than " + p2 + ", but has a more general path pattern"
		}
		return "http: pattern " + strconv.Quote(p1) + " conflicts with pattern " + strconv.Quote(p2) + ":\n" + why
	}
	return ""
}

// addRoute faults when nr conflicts with a route of r (ServeMux.Handle's
// panic, without the registration sites), else adds it.
func addRoute(r *Http_Router, nr *route) {
	if e := conflictError(r.routes.s, nr); e != "" {
		FaultHere("%s", e)
	}
	List_push(nil, &r.routes, nr)
}
