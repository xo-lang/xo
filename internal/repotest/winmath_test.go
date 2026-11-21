package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestSqliteMathFunctionsDefinedOnWindows (task #621): every C math
// function std/sqlite's SQLITE_ENABLE_MATH_FUNCTIONS table calls (xw
// decision 251) has a definition in Xo's no-C-runtime Windows C library
// (std/native/llrt/cabi_wincrt.xo, cabi_winmath.xo). The 0.56.0 recut of
// the windows prebuilt objects failed scripts/sqlite-prebuilt.sh's symbol
// check: "object needs symbols nothing defines: acos acosh asin ... trunc".
func TestSqliteMathFunctionsDefinedOnWindows(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(root, "std/native/sqlite/sqlite3.c"))
	if err != nil {
		t.Fatal(err)
	}
	defined := map[string]bool{}
	fnRe := regexp.MustCompile(`(?m)^fn ([A-Za-z_][A-Za-z0-9_]*)\(`)
	for _, f := range []string{"cabi_wincrt.xo", "cabi_winmath.xo"} {
		b, err := os.ReadFile(filepath.Join(root, "std/native/llrt", f))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		for _, m := range fnRe.FindAllStringSubmatch(string(b), -1) {
			defined[m[1]] = true
		}
	}
	// MFUNCTION(zName, nArg, xPtr, xFunc): xPtr is the C function, or one
	// of sqlite's own wrappers or 0; xCeil and xFloor are ceil and floor
	re := regexp.MustCompile(`MFUNCTION\(\s*\w+\s*,\s*\d+\s*,\s*(\w+)\s*,`)
	alias := map[string]string{"xCeil": "ceil", "xFloor": "floor"}
	own := map[string]bool{"degToRad": true, "radToDeg": true, "0": true, "xPtr": true}
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		n := m[1]
		if a, ok := alias[n]; ok {
			n = a
		}
		if !own[n] {
			seen[n] = true
		}
	}
	if len(seen) < 15 {
		t.Fatalf("found %d math functions in sqlite3.c's table; the pattern no longer matches", len(seen))
	}
	var missing []string
	for n := range seen {
		if !defined[n] {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("C math functions sqlite calls that no-CRT Windows programs do not define: %s", strings.Join(missing, " "))
	}
}
