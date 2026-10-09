package mod

import (
	"fmt"
	"strconv"
	"strings"
)

// MajorSuffix splits a /vN major version suffix (N >= 2) off a module
// path: "github.com/acme/billing/v2" is ("github.com/acme/billing", 2).
// A path without one is (path, 0). Major versions 0 and 1 have no suffix
// (core 4.2, as in Go).
func MajorSuffix(path string) (string, int) {
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return path, 0
	}
	last := path[i+1:]
	if len(last) < 2 || last[0] != 'v' || last[1] == '0' {
		return path, 0
	}
	n, err := strconv.Atoi(last[1:])
	if err != nil || n < 2 || strconv.Itoa(n) != last[1:] {
		return path, 0
	}
	return path[:i], n
}

// Major is the major version number of a valid version.
func Major(v string) int {
	m := versionRE.FindStringSubmatch(v)
	if m == nil {
		return -1
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// MajorMismatch describes why version v cannot be a version of module
// path ("" when it can): a path ending in /vN takes only vN.x.y tags, a
// path without a suffix only v0 and v1.
func MajorMismatch(path, v string) string {
	base, n := MajorSuffix(path)
	m := Major(v)
	switch {
	case m < 0:
		return ""
	case n >= 2 && m != n:
		return fmt.Sprintf("%s %s: a module path ending in /v%d takes only v%d.x.y versions", path, v, n, n)
	case n == 0 && m >= 2:
		return fmt.Sprintf("%s %s: major version %d needs the module path %s/v%d", path, v, m, base, m)
	}
	return ""
}

// repoPath is the path whose git location holds module path: the path
// without its /vN suffix (all major versions share one repository).
func repoPath(path string) string {
	base, _ := MajorSuffix(path)
	return base
}
