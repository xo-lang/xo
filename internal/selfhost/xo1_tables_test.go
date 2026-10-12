package selfhost

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestXo1IOFlagsSymbols checks that xo1's copy of build.IOFlags
// (selfhost/xo/link.xo io_flags) names the same runtime symbols as the Go
// driver, so both compile the runtime with the same -D flags
// (gotchas/xo1-duplicated-tables-drift.md, decision 0178).
func TestXo1IOFlagsSymbols(t *testing.T) {
	goSyms := bodySymbols(t, "../build/tzdata.go", "func IOFlags(", "\n}\n")
	xoSyms := bodySymbols(t, "../../selfhost/xo/link.xo", "fn io_flags(", "\n}\n")
	if len(goSyms) == 0 {
		t.Fatal("no symbols found in build.IOFlags")
	}
	if strings.Join(goSyms, " ") != strings.Join(xoSyms, " ") {
		t.Errorf("io_flags differs from build.IOFlags:\n Go: %v\n Xo: %v", goSyms, xoSyms)
	}
}

var runtimeSym = regexp.MustCompile(`"(xo_[a-z0-9_]+)"`)

// bodySymbols returns the sorted xo_ symbols quoted in the function of
// file that starts at start and ends at the first end after it.
func bodySymbols(t *testing.T, file, start, end string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	i := strings.Index(s, start)
	if i < 0 {
		t.Fatalf("%s: %q not found", file, start)
	}
	s = s[i:]
	if j := strings.Index(s, end); j >= 0 {
		s = s[:j]
	}
	var out []string
	for _, m := range runtimeSym.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}
