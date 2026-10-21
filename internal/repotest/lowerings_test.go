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

// lowerGate matches a check of whether an optional SSA builder lowering is
// on: b.lowering_on("NAME").
var lowerGate = regexp.MustCompile(`\blowering_on\("([a-z0-9_]+)"\)`)

// TestLoweringsListed fails when a lowering the SSA builder gates by
// $XO_LOWER is not in compiler/ssa lowerings.xo's lowerings(), or a listed
// one is gated nowhere: XO_LOWER rejects a name lowerings() does not list,
// so an unlisted lowering could not be turned off, and a listed one that
// nothing checks would turn off nothing (2026-10-21, as TestPassesListed).
func TestLoweringsListed(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(root, "compiler", "ssa", "lowerings.xo"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	i := strings.Index(body, "pub fn lowerings()")
	j := strings.Index(body[i:], "\n}\n")
	if i < 0 || j < 0 {
		t.Fatal("no lowerings() in compiler/ssa/lowerings.xo")
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(body[i:i+j], "\n") {
		if c := strings.Index(line, "//"); c >= 0 {
			line = line[:c]
		}
		for _, m := range regexp.MustCompile(`"([a-z0-9_]+)"`).FindAllStringSubmatch(line, -1) {
			listed[m[1]] = true
		}
	}
	gated := map[string]string{}
	err = filepath.WalkDir(filepath.Join(root, "compiler", "ssa"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".xo") || strings.HasSuffix(path, "_test.xo") || strings.HasSuffix(path, "lowerings.xo") {
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
			for _, m := range lowerGate.FindAllStringSubmatch(line, -1) {
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
		t.Errorf("lowerings gated by XO_LOWER but not in compiler/ssa lowerings.xo lowerings(): %s", strings.Join(missing, ", "))
	}
	if len(unused) > 0 {
		t.Errorf("lowerings in lowerings() that nothing checks (turning them off does nothing): %s", strings.Join(unused, ", "))
	}
}
