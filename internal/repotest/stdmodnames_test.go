package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// stdFnRE matches a top level function or method declaration's name and
// receiver type.
var stdFnRE = regexp.MustCompile(`(?m)^(?:pub )?fn (?:\((?:var )?[a-z_][a-z_0-9]*: ?(?:var )?([A-Za-z_][A-Za-z_0-9.]*)[^)]*\) )?([a-z_][a-z_0-9]*)`)

// stdFuncNames returns the functions ("name") and methods ("Type.name")
// the files declare.
func stdFuncNames(t *testing.T, files ...string) map[string]bool {
	out := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range stdFnRE.FindAllStringSubmatch(string(b), -1) {
			name := m[2]
			if m[1] != "" {
				recv := m[1][strings.LastIndex(m[1], ".")+1:]
				name = recv + "." + name
			}
			out[name] = true
		}
	}
	return out
}

// TestStdSameNameModulesDisjoint: a std module (std/NAME.xo with its
// body std/native/NAME.xo) and a runtime module (std/native/llrt/NAME.xo)
// of the same name may not declare the same function. The SSA names a
// module's functions by its default name, the last path segment (a
// program module that shares one is named by its path instead, ssa
// path_named_mods), and the std store keeps std's names whatever a
// build loads, so `x509.parse` of two std modules would be one function:
// a call bound to either body (2026-10-29, xo-lang-5's std/x509).
func TestStdSameNameModulesDisjoint(t *testing.T) {
	llrt, err := filepath.Glob(filepath.Join(root, "std", "native", "llrt", "*.xo"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, lf := range llrt {
		if strings.HasSuffix(lf, "_test.xo") {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(lf), ".xo")
		top := filepath.Join(root, "std", name+".xo")
		if _, err := os.Stat(top); err != nil {
			continue
		}
		checked++
		a := stdFuncNames(t, top, filepath.Join(root, "std", "native", name+".xo"))
		b := stdFuncNames(t, lf)
		var both []string
		for n := range a {
			if b[n] {
				both = append(both, n)
			}
		}
		sort.Strings(both)
		if len(both) > 0 {
			t.Errorf("std/%s.xo and std/native/llrt/%s.xo both declare %s: their SSA names are one (`%s.%s`); rename one (or the llrt module)", name, name, strings.Join(both, ", "), name, both[0])
		}
	}
	t.Logf("%d std modules share a name with a runtime module", checked)
}
