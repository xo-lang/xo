package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	waitsListRE  = regexp.MustCompile(`(?m)^const (WAITS|NO_WAIT) = \[(.*)\]$`)
	waitsIfaceRE = regexp.MustCompile(`^(?:pub )?interface (\w+)`)
	waitsFnRE    = regexp.MustCompile(`^\s*(?:pub )?fn (?:\(\w+: (?:var )?(\w+)(?:\[[^\]]*\])?\) )?(\w+)`)
	waitsUsesRE  = regexp.MustCompile(`\buses (.*)$`)
	waitsEffRE   = regexp.MustCompile(`\b(clock|stdio|proc|fs)\b`)
	// std/testing's capabilities lower as the prelude's (ssa build.xo
	// test_cap), under the same keys
	waitsTestCap = map[string]string{"MemFs": "Fs", "FakeClock": "Clock", "FakeStdio": "Stdio", "FakeProc": "Proc"}
)

// TestWaitsClassified checks compiler/ssa/waits.xo's lists against the
// declared effects of std's surface: every std function with no Xo body
// (a declaration in std/*.xo, implemented by the native prelude or a
// native module) whose `uses` names clock, stdio, proc or fs is in WAITS
// (it waits on the event loop, so a program calling it keeps the
// scheduler, concflag.xo) or NO_WAIT, and every name the lists hold is
// such a function. `net` needs no entry: a std function declaring it
// counts as waiting from its effect (waits.xo call_waits), so a new
// network module cannot be missed (gotchas/conc-count-by-name-lists.md).
func TestWaitsClassified(t *testing.T) {
	root := "../.."
	src, err := os.ReadFile(filepath.Join(root, "compiler/ssa/waits.xo"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]string{}
	for _, m := range waitsListRE.FindAllStringSubmatch(string(src), -1) {
		for _, q := range strings.Split(m[2], ",") {
			name := strings.Trim(strings.TrimSpace(q), `"`)
			if prev, ok := listed[name]; ok {
				t.Errorf("%s is in both %s and %s", name, prev, m[1])
			}
			listed[name] = m[1]
		}
	}
	if len(listed) == 0 {
		t.Fatal("no WAITS or NO_WAIT list in compiler/ssa/waits.xo")
	}
	files, _ := filepath.Glob(filepath.Join(root, "std", "*.xo"))
	declared := map[string]string{}
	for _, p := range files {
		if strings.HasSuffix(p, "_test.xo") {
			continue
		}
		mod := strings.TrimSuffix(filepath.Base(p), ".xo")
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		iface := ""
		for _, line := range strings.Split(string(data), "\n") {
			if m := waitsIfaceRE.FindStringSubmatch(line); m != nil {
				iface = m[1]
				continue
			}
			if iface != "" && strings.HasPrefix(line, "}") {
				iface = ""
				continue
			}
			m := waitsFnRE.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			u := waitsUsesRE.FindStringSubmatch(line)
			if u == nil || strings.Contains(u[1], "{") || !waitsEffRE.MatchString(u[1]) {
				continue
			}
			recv := m[1]
			if recv == "" {
				recv = iface
			}
			if recv == "" {
				recv = mod
			}
			if c, ok := waitsTestCap[recv]; ok && mod == "testing" {
				recv = c
			}
			if _, ok := declared[recv+"."+m[2]]; !ok {
				declared[recv+"."+m[2]] = filepath.Base(p)
			}
		}
	}
	var missing, stale []string
	for name, file := range declared {
		if _, ok := listed[name]; !ok {
			missing = append(missing, name+" ("+file+")")
		}
	}
	for name := range listed {
		if _, ok := declared[name]; !ok {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	for _, n := range missing {
		t.Errorf("%s declares clock, stdio, proc or fs with no Xo body: add it to compiler/ssa/waits.xo WAITS if a call waits on the event loop (sleep, a read of standard input, a child process, a lock), else to NO_WAIT", n)
	}
	for _, n := range stale {
		t.Errorf("compiler/ssa/waits.xo lists %s, which is no std function with no Xo body declaring clock, stdio, proc or fs: remove it", n)
	}
}
