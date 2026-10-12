package selfhost

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// modFixtures are the module trees TestSelfhostCheckerModuleMutations
// mutates: a main module with dependencies in the fixture module cache,
// a workspace, a vendored build, and deny rules (decisions 0086, 0095).
var modFixtures = []string{
	"internal/selfhost/testdata/modules/app",
	"internal/check/testdata/golden/effect_ceiling_work",
	"internal/check/testdata/golden/effect_ceiling_vendor",
	"internal/check/testdata/golden/imports_deny",
}

// TestSelfhostCheckerModuleMutations compares the Xo and Go checkers
// (decision 0141) on mutations of module files: an effect ceiling lowered,
// raised, added, or malformed (xo.mod require lines and xo.work use
// lines), a deny rule added (valid, naming nothing, or malformed), a
// require version changed (another cached version, one missing from the
// cache, an invalid one), an xo.lock or vendor/modules.txt line corrupted
// or removed, and a `use` of another package added to a source file. Each
// mutation copies one fixture tree; every package directory of the copy
// is a run, compared as TestSelfhostChecker compares runs. -short makes
// 40 mutations, else 400 (tier_test.go);
// XO_SELFHOST_SEED and XO_SELFHOST_MOD_FUZZ=N
// override. The Xo checker is built with the go backend, or the one
// XO_SELFHOST_BACKEND names.
func TestSelfhostCheckerModuleMutations(t *testing.T) {
	n := byTier(t, 40, 400, 400)
	if v := os.Getenv("XO_SELFHOST_MOD_FUZZ"); v != "" {
		fmt.Sscan(v, &n)
	}
	seed := int64(20261011)
	if v := os.Getenv("XO_SELFHOST_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	root := repoRoot(t)
	backend := "go"
	if v := os.Getenv("XO_SELFHOST_BACKEND"); v != "" {
		backend = v
	}
	bin, err := selfhostBinary("check", backend)
	if err != nil {
		t.Fatal(err)
	}
	stdDir, err := filepath.Abs(filepath.Join(root, "std"))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if keep := os.Getenv("XO_SELFHOST_KEEP"); keep != "" {
		tmp = filepath.Join(keep, "check-module-mutations")
		os.RemoveAll(tmp)
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	rng := rand.New(rand.NewSource(seed))
	var dirs []string
	kinds := map[string]int{}
	for i := range n {
		fix := modFixtures[rng.Intn(len(modFixtures))]
		name := fmt.Sprintf("m%04d", i)
		dst := filepath.Join(tmp, name)
		copyDir(t, filepath.Join(root, fix), dst)
		kinds[mutateModule(t, rng, dst)]++
		for _, d := range xoDirs(dst) {
			rel, _ := filepath.Rel(tmp, d)
			dirs = append(dirs, filepath.ToSlash(rel))
		}
	}
	t.Chdir(tmp)
	setModEnv(t, root)
	if goldensRecording() {
		for _, d := range dirs {
			goCheck(d)
		}
		skipRecording(t)
	}
	got := map[string]string{}
	const batch = 100
	for i := 0; i < len(dirs); i += batch {
		chunk := dirs[i:min(i+batch, len(dirs))]
		text := runXo(t, bin, tmp, append([]string{"--each", "--std=" + stdDir, "--wd=" + workDir(t)}, chunk...))
		parts := strings.Split(text, "=== check ")
		if len(parts) != len(chunk)+1 {
			t.Fatalf("Xo checker printed %d runs for %d directories", len(parts)-1, len(chunk))
		}
		for j, d := range chunk {
			got[d] = strings.TrimPrefix(parts[j+1], d+"\n")
		}
	}
	var st checkStats
	var fails []string
	codes := map[string]int{}
	others := map[string]int{}
	for _, d := range dirs {
		want := goCheck(d)
		before, otherBefore := st.stagedOK, st.otherRejected
		if msg := compareRun(d, want, got[d], &st); msg != "" {
			fails = append(fails, msg)
		}
		if st.stagedOK > before {
			for _, l := range want.stagedLines(got[d]) {
				codes[l[len(`{"code":"`):len(`{"code":"XO0000`)]]++
			}
		}
		if st.otherRejected > otherBefore {
			for _, code := range want.other {
				others[code]++
			}
		}
	}
	t.Logf("%d module mutations (%v), %d runs: %d accepted by Go (%d match), %d rejected only with diagnostics of the stages done (%d match), %d rejected otherwise, %d unsupported, %d not loadable",
		n, kinds, len(dirs), st.accepted, st.acceptedOK, st.staged, st.stagedOK, st.otherRejected, st.unsupported, st.errors)
	t.Logf("diagnostics of the matching rejected runs: %v", codes)
	t.Logf("the other errors of the runs rejected otherwise: %v", others)
	if len(fails) > 0 {
		if len(fails) > 5 {
			fails = append(fails[:5], fmt.Sprintf("... and %d more", len(fails)-5))
		}
		t.Fatalf("%d of %d runs differ:\n%s", len(fails), len(dirs), strings.Join(fails, "\n"))
	}
	if st.acceptedOK+st.stagedOK < len(dirs)*9/10 {
		t.Fatalf("only %d of %d runs were compared", st.acceptedOK+st.stagedOK, len(dirs))
	}
}

// copyDir copies the tree at src to dst.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// xoDirs lists the directories under root that hold .xo files.
func xoDirs(root string) []string {
	seen := map[string]bool{}
	var out []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".xo") && !seen[filepath.Dir(p)] {
			seen[filepath.Dir(p)] = true
			out = append(out, filepath.Dir(p))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// files lists the files under root with the given base name.
func filesNamed(root, name string) []string {
	var out []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

var (
	ceilingLineRE = regexp.MustCompile(`(?m)^(require [^ \n]+ [^ \n]+|use [^ \n]+)( uses [^\n]*)?$`)
	requireLineRE = regexp.MustCompile(`(?m)^require ([^ \n]+) ([^ \n]+)`)
	lockLineRE    = regexp.MustCompile(`(?m)^[^#\n][^\n]*$`)
	vendorLineRE  = regexp.MustCompile(`(?m)^# [^\n]*$`)
	modulePathRE  = regexp.MustCompile(`(?m)^module ([^ \n]+)`)
)

// mutateModule applies one module mutation to the tree at dir and names
// its kind.
func mutateModule(t *testing.T, rng *rand.Rand, dir string) string {
	t.Helper()
	for range 20 {
		var kind string
		var ok bool
		switch rng.Intn(6) {
		case 0, 1:
			kind, ok = "ceiling", mutateCeiling(rng, dir)
		case 2:
			kind, ok = "deny", mutateDeny(rng, dir)
		case 3:
			kind, ok = "version", mutateVersion(rng, dir)
		case 4:
			kind, ok = "lock", mutateLock(rng, dir)
		default:
			kind, ok = "use", mutateUse(rng, dir)
		}
		if ok {
			return kind
		}
	}
	return "none"
}

// edit rewrites a file through f; false when f changes nothing.
func edit(path string, f func(string) string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	out := f(string(data))
	if out == string(data) {
		return false
	}
	return os.WriteFile(path, []byte(out), 0o644) == nil
}

// pick replaces the match of re chosen by rng in s with f of it.
func pick(rng *rand.Rand, s string, re *regexp.Regexp, f func(m []string) string) string {
	ms := re.FindAllStringSubmatchIndex(s, -1)
	if len(ms) == 0 {
		return s
	}
	m := ms[rng.Intn(len(ms))]
	var groups []string
	for i := 0; i < len(m); i += 2 {
		if m[i] < 0 {
			groups = append(groups, "")
		} else {
			groups = append(groups, s[m[i]:m[i+1]])
		}
	}
	return s[:m[0]] + f(groups) + s[m[1]:]
}

// mainModFiles are the xo.mod files of the tree outside vendor/.
func mainModFiles(dir string) []string {
	var out []string
	for _, p := range filesNamed(dir, "xo.mod") {
		if !strings.Contains(filepath.ToSlash(p), "/vendor/") {
			out = append(out, p)
		}
	}
	return out
}

func choose(rng *rand.Rand, xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[rng.Intn(len(xs))]
}

var ceilings = []string{" uses none", " uses clock", " uses rand", " uses fs", " uses fs.read", " uses clock, rand", " uses (clock)", " uses net.dial", " uses", " uses bogus", " uses none, clock"}

// mutateCeiling lowers, raises, adds, or garbles the effect ceiling of a
// require line or an xo.work use line.
func mutateCeiling(rng *rand.Rand, dir string) bool {
	files := append(mainModFiles(dir), filesNamed(dir, "xo.work")...)
	return edit(choose(rng, files), func(s string) string {
		return pick(rng, s, ceilingLineRE, func(m []string) string {
			line, clause := m[1], m[2]
			switch {
			case clause != "" && rng.Intn(3) == 0:
				return line // raised: no ceiling
			case clause != "" && strings.Contains(clause, ",") && rng.Intn(2) == 0:
				labels := strings.Split(strings.TrimPrefix(clause, " uses "), ",")
				labels = append(labels[:rng.Intn(len(labels))], labels[rng.Intn(len(labels))+1:]...)
				if len(labels) == 0 {
					return line + " uses none"
				}
				return line + " uses " + strings.TrimSpace(strings.Join(labels, ","))
			}
			return line + choose(rng, ceilings)
		})
	})
}

// packages are the package paths of the tree's main modules, their
// dependencies, a few std modules, and some that do not exist.
func packages(dir string) []string {
	out := []string{"std/time", "std/json", "std/nosuch", "std", "example.com/nope", "example.com/app/missing"}
	for _, mf := range mainModFiles(dir) {
		data, _ := os.ReadFile(mf)
		m := modulePathRE.FindStringSubmatch(string(data))
		if m == nil {
			continue
		}
		out = append(out, m[1])
		for _, r := range requireLineRE.FindAllStringSubmatch(string(data), -1) {
			out = append(out, r[1])
		}
		mdir := filepath.Dir(mf)
		for _, d := range xoDirs(mdir) {
			rel, _ := filepath.Rel(mdir, d)
			if rel != "." && !strings.HasPrefix(filepath.ToSlash(rel), "vendor/") {
				out = append(out, m[1]+"/"+filepath.ToSlash(rel))
			}
		}
	}
	return out
}

// mutateDeny adds a deny rule to an xo.mod: between two packages, with
// `/...` patterns, naming nothing, or malformed.
func mutateDeny(rng *rand.Rand, dir string) bool {
	pkgs := packages(dir)
	pat := func() string {
		p := choose(rng, pkgs)
		if rng.Intn(3) == 0 {
			p += "/..."
		}
		return p
	}
	var rule string
	switch rng.Intn(8) {
	case 0:
		rule = "deny " + pat() + " " + pat()
	case 1:
		rule = "deny " + pat() + " -> " + pat() + ", ../x"
	default:
		rule = "deny " + pat() + " -> " + pat()
		if rng.Intn(2) == 0 {
			rule += ", " + pat()
		}
	}
	return edit(choose(rng, mainModFiles(dir)), func(s string) string {
		if rng.Intn(2) == 0 {
			return s + rule + "\n"
		}
		return s + "\n" + rule + " // added\n"
	})
}

var versions = []string{"v1.0.0", "v1.1.0", "v1.2.0", "v0.3.0", "v1.0.1", "v2.0.0", "v1.0", "v1.0.0-rc.1", "v01.0.0"}

// mutateVersion changes the version of a require line.
func mutateVersion(rng *rand.Rand, dir string) bool {
	return edit(choose(rng, mainModFiles(dir)), func(s string) string {
		return pick(rng, s, requireLineRE, func(m []string) string {
			return "require " + m[1] + " " + choose(rng, versions)
		})
	})
}

// mutateLock corrupts, removes, or garbles an xo.lock or
// vendor/modules.txt line.
func mutateLock(rng *rand.Rand, dir string) bool {
	files := append(filesNamed(dir, "xo.lock"), filesNamed(dir, "modules.txt")...)
	path := choose(rng, files)
	re := lockLineRE
	if filepath.Base(path) == "modules.txt" {
		re = vendorLineRE
	}
	return edit(path, func(s string) string {
		return pick(rng, s, re, func(m []string) string {
			line := m[0]
			switch rng.Intn(4) {
			case 0:
				return ""
			case 1:
				return line + " extra"
			case 2:
				return strings.Replace(line, "v1.", "v9.", 1)
			}
			if i := strings.Index(line, "h1:"); i >= 0 && i+5 < len(line) {
				b := []byte(line)
				if b[i+5] == 'A' {
					b[i+5] = 'B'
				} else {
					b[i+5] = 'A'
				}
				return string(b)
			}
			return line + "0"
		})
	})
}

// mutateUse adds a `use` of another package to a source file of a main
// module.
func mutateUse(rng *rand.Rand, dir string) bool {
	var srcs []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".xo") && !strings.Contains(filepath.ToSlash(p), "/vendor/") {
			srcs = append(srcs, p)
		}
		return nil
	})
	pkg := choose(rng, packages(dir))
	return edit(choose(rng, srcs), func(s string) string {
		return "use " + pkg + " as m_added\n" + s
	})
}
