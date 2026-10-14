package modtest

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The module cache layout and version order the tests need (copied from
// the Go internal/mod when decision 0215 deleted it; the self hosted
// selfhost/cli/modules and selfhost/check implement them for xo).

var versionRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)

// Compare orders two semantic versions vX.Y.Z[-pre]: -1, 0, or 1. A
// prerelease sorts before its release; prereleases compare by their dot
// separated parts. Strings that are not versions compare as strings.
func Compare(a, b string) int {
	ma, mb := versionRE.FindStringSubmatch(a), versionRE.FindStringSubmatch(b)
	if ma == nil || mb == nil {
		return strings.Compare(a, b)
	}
	for i := 1; i <= 3; i++ {
		if c := cmpNum(ma[i], mb[i]); c != 0 {
			return c
		}
	}
	pa, pb := strings.TrimPrefix(ma[4], "-"), strings.TrimPrefix(mb[4], "-")
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	}
	xa, xb := strings.Split(pa, "."), strings.Split(pb, ".")
	for i := 0; i < len(xa) && i < len(xb); i++ {
		na, ea := strconv.Atoi(xa[i])
		nb, eb := strconv.Atoi(xb[i])
		switch {
		case ea == nil && eb == nil:
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
		case ea == nil:
			return -1
		case eb == nil:
			return 1
		default:
			if c := strings.Compare(xa[i], xb[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(xa) < len(xb):
		return -1
	case len(xa) > len(xb):
		return 1
	}
	return 0
}

func cmpNum(a, b string) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// CacheDir is the module cache: $XO_MODCACHE, else <user cache>/xo/mod.
func CacheDir() string {
	if d := os.Getenv("XO_MODCACHE"); d != "" {
		if abs, err := filepath.Abs(d); err == nil {
			return abs
		}
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "xo", "mod")
	}
	return filepath.Join(os.TempDir(), "xo-mod")
}

// VersionDir is the cache directory of a module version (it may not
// exist): upper case letters of the path escaped as `!` and the lower
// case letter.
func VersionDir(path, version string) string {
	var b strings.Builder
	for _, r := range path {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return filepath.Join(CacheDir(), filepath.FromSlash(b.String())+"@"+version)
}

// CleanCache removes the module cache (read only trees made writable
// first).
func CleanCache() error {
	dir := CacheDir()
	MakeWritable(dir)
	return os.RemoveAll(dir)
}
