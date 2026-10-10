package build

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/check"
)

// cInputs writes the C adapters of the program's `use c` modules
// (internal/cbind, decision 0116) into dir/cffi and returns the C
// sources, objects, and archives to compile and link with the program,
// and the flags: -l for each library (ccBuild moves them after the
// sources). An adapter includes a module's header by its absolute path
// and a `link` source finds its headers next to it, so no -I is needed
// (which would turn off the runtime object cache, rtobj.go). A `link`
// file of the module lies outside dir, so such a build is not cached
// (nativeBinKey); a build that only links system libraries is.
func cInputs(res *check.Result, dir string) (srcs, flags []string, err error) {
	seen := map[string]bool{}
	libs := map[string]bool{}
	var mods []*check.Module
	for _, m := range res.CModules() {
		if m.C != nil && !seen[m.C.Key] {
			seen[m.C.Key] = true
			mods = append(mods, m)
		}
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].C.Prefix < mods[j].C.Prefix })
	if len(mods) == 0 {
		return nil, nil, nil
	}
	cdir := filepath.Join(dir, "cffi")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		return nil, nil, err
	}
	for _, m := range mods {
		b := m.C
		p := filepath.Join(cdir, "xo_"+strings.TrimSuffix(b.Prefix, "_")+".c")
		if old, err := os.ReadFile(p); err != nil || string(old) != string(b.Adapter) {
			if err := os.WriteFile(p, b.Adapter, 0o644); err != nil {
				return nil, nil, err
			}
		}
		srcs = append(srcs, p)
		for _, f := range b.LinkFiles {
			if !seen[f] {
				seen[f] = true
				srcs = append(srcs, f)
			}
		}
		for _, l := range b.LinkLibs {
			libs[l] = true
		}
	}
	for _, l := range sortedSet(libs) {
		flags = append(flags, "-l"+l)
	}
	return srcs, flags, nil
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// splitLibs separates -l flags, which the linker wants after the
// objects that use them, from the others.
func splitLibs(flags []string) (other, libs []string) {
	for _, f := range flags {
		if strings.HasPrefix(f, "-l") && len(f) > 2 {
			libs = append(libs, f)
			continue
		}
		other = append(other, f)
	}
	return other, libs
}
