package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Decision 0341: the runtime's record layouts that more than one module
// uses live in std/native/llrt/layout.xo, a leaf module. These checks
// keep copies honest while records move there.
var (
	layoutConst = regexp.MustCompile(`^\s*(pub\s+)?const\s+([A-Z][A-Z0-9_]*)\s*(?::\s*\w+\s*)?=\s*(-?\d+|[a-z_][a-z0-9_]*\.[A-Z][A-Z0-9_]*)\s*(//.*)?$`)
	layoutMark  = regexp.MustCompile(`//\s*=\s*([a-z_][a-z0-9_]*)\.([A-Z][A-Z0-9_]*)\b`)
	layoutOwn   = regexp.MustCompile(`//\s*own:`)
	layoutUse   = regexp.MustCompile(`^\s*use\s+(\S+)`)
)

type llrtConst struct {
	file, name, expr, mark string
	pub, own               bool
	line                   int
}

// llrtConsts reads every constant of std/native/llrt's non-test files
// with an integer literal or a `module.NAME` value, by module.
func llrtConsts(t *testing.T) map[string][]llrtConst {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "std", "native", "llrt", "*.xo"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no std/native/llrt sources: %v", err)
	}
	mods := map[string][]llrtConst{}
	for _, p := range files {
		if strings.HasSuffix(p, "_test.xo") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		mod := strings.TrimSuffix(filepath.Base(p), ".xo")
		for i, l := range strings.Split(string(b), "\n") {
			m := layoutConst.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			c := llrtConst{file: filepath.Base(p), name: m[2], expr: m[3], pub: m[1] != "", own: layoutOwn.MatchString(m[4]), line: i + 1}
			if mm := layoutMark.FindStringSubmatch(m[4]); mm != nil {
				c.mark = mm[1] + "." + mm[2]
			}
			mods[mod] = append(mods[mod], c)
		}
	}
	return mods
}

// TestRuntimeLayoutCopies (decision 0341):
//   - layout.xo imports nothing (every module can use it, so none needs
//     a copy for an import cycle's sake), and each record's fields lie
//     below its size (NAME_SIZE for the fields named NAME_*);
//   - a constant marked `// = module.NAME` has that constant's value;
//   - no other module defines a name layout.xo declares with another
//     value unless marked `// own:` (a local constant that only shares
//     the name): fault.xo's C_TRACE was ctx's C_FAULT_TRACE under the
//     name of ctx's C_TRACE.
func TestRuntimeLayoutCopies(t *testing.T) {
	mods := llrtConsts(t)
	val := map[string]int64{}
	// resolve module.NAME values (aliases chain through other modules)
	var resolve func(mod, name string, depth int) (int64, bool)
	resolve = func(mod, name string, depth int) (int64, bool) {
		if depth > 8 {
			return 0, false
		}
		for _, c := range mods[mod] {
			if c.name != name {
				continue
			}
			if n, err := strconv.ParseInt(c.expr, 10, 64); err == nil {
				return n, true
			}
			if i := strings.IndexByte(c.expr, '.'); i > 0 {
				return resolve(c.expr[:i], c.expr[i+1:], depth+1)
			}
		}
		return 0, false
	}
	for mod, cs := range mods {
		for _, c := range cs {
			if v, ok := resolve(mod, c.name, 0); ok {
				val[mod+"."+c.name] = v
			}
		}
	}
	lay, ok := mods["layout"]
	if !ok {
		t.Fatal("no std/native/llrt/layout.xo (decision 0341)")
	}
	b, err := os.ReadFile(filepath.Join(root, "std", "native", "llrt", "layout.xo"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		if m := layoutUse.FindStringSubmatch(l); m != nil {
			t.Errorf("layout.xo uses %s: it must import nothing, so any module can use it without an import cycle (decision 0341)", m[1])
		}
	}
	// sizes: every NAME_* field of a record below its NAME_SIZE
	sizes := map[string]int64{}
	for _, c := range lay {
		if strings.HasSuffix(c.name, "_SIZE") {
			sizes[strings.TrimSuffix(c.name, "_SIZE")] = val["layout."+c.name]
		}
	}
	for _, c := range lay {
		if !c.pub {
			t.Errorf("layout.xo:%d: %s is not pub: layout.xo declares what other modules use", c.line, c.name)
		}
		i := strings.IndexByte(c.name, '_')
		if i < 0 || strings.HasSuffix(c.name, "_SIZE") || strings.HasSuffix(c.name, "_W") {
			continue
		}
		if size, ok := sizes[c.name[:i]]; ok && val["layout."+c.name] >= size {
			t.Errorf("layout.xo:%d: %s = %d is at or past %s_SIZE = %d: grow the record's size with the field", c.line, c.name, val["layout."+c.name], c.name[:i], size)
		}
	}
	var errs []string
	for mod, cs := range mods {
		for _, c := range cs {
			v, ok := val[mod+"."+c.name]
			if c.mark != "" {
				want, found := val[c.mark]
				if !found {
					errs = append(errs, c.file+":"+strconv.Itoa(c.line)+": "+c.name+" is marked a copy of "+c.mark+", which is not an integer constant of llrt")
				} else if !ok || v != want {
					errs = append(errs, c.file+":"+strconv.Itoa(c.line)+": "+c.name+" = "+strconv.FormatInt(v, 10)+" copies "+c.mark+" = "+strconv.FormatInt(want, 10))
				}
				continue
			}
			if mod == "layout" || c.own || !ok {
				continue
			}
			if lv, declared := val["layout."+c.name]; declared && lv != v && strings.TrimSpace(c.expr) != "layout."+c.name {
				errs = append(errs, c.file+":"+strconv.Itoa(c.line)+": "+c.name+" = "+strconv.FormatInt(v, 10)+" but layout.xo declares "+c.name+" = "+strconv.FormatInt(lv, 10)+": use layout."+c.name+", or rename it, or mark it `// own: why`")
			}
		}
	}
	sort.Strings(errs)
	for _, e := range errs {
		t.Error(e)
	}
}
