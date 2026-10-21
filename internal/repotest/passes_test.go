package repotest

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// passGate matches a check of whether an optional pass is on: the arm64
// emitter's g.on("NAME"), passes.xo's pass_on(m, "NAME"), and a direct
// passes_off.contains("NAME").
var passGate = regexp.MustCompile(`(?:\bg\.on\(|\bpass_on\([^,()]+, |\bpasses_off\.contains\()"([a-z0-9_]+)"\)`)

// TestPassesListed fails when a pass the compiler gates by $XO_PASSES is
// not in compiler/arm64 passes.xo's passes(), or a listed one is gated
// nowhere: XO_PASSES rejects a name passes() does not list, so an
// unlisted pass could not be turned off, and gate.sh's alloff step (every
// pass in passes() off) would leave it on (2026-10-21, the lead).
func TestPassesListed(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(root, "compiler", "arm64", "passes.xo"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	i := strings.Index(body, "pub fn passes()")
	j := strings.Index(body[i:], "\n}\n")
	if i < 0 || j < 0 {
		t.Fatal("no passes() in compiler/arm64/passes.xo")
	}
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile(`"([a-z0-9_]+)"`).FindAllStringSubmatch(body[i:i+j], -1) {
		listed[m[1]] = true
	}
	gated := map[string]string{}
	err = filepath.WalkDir(filepath.Join(root, "compiler"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".xo") || strings.HasSuffix(path, "_test.xo") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for n, line := range strings.Split(string(data), "\n") {
			if c := strings.Index(line, "//"); c >= 0 {
				line = line[:c]
			}
			for _, m := range passGate.FindAllStringSubmatch(line, -1) {
				if _, ok := gated[m[1]]; !ok {
					gated[m[1]] = filepath.ToSlash(rel) + ":" + itoa(n+1)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var missing, unused []string
	for n, at := range gated {
		if !listed[n] {
			missing = append(missing, n+" ("+at+")")
		}
	}
	for n := range listed {
		if _, ok := gated[n]; !ok {
			unused = append(unused, n)
		}
	}
	sort.Strings(missing)
	sort.Strings(unused)
	if len(missing) > 0 {
		t.Errorf("passes gated by XO_PASSES but not in compiler/arm64 passes.xo passes(): %s", strings.Join(missing, ", "))
	}
	if len(unused) > 0 {
		t.Errorf("passes in passes() that nothing checks (turning them off does nothing): %s", strings.Join(unused, ", "))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
