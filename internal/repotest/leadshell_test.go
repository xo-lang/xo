package repotest

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// scripts/lead/shelltests.txt lists the scripts/lead/*_test.sh files that
// TestLeadShell runs, each with the path prefixes of the files it runs
// (task #588, 2026-11-21: 55 of those tests had no caller, and
// pushq_prelane_test.sh had failed since #579 changed what a std test
// file push needs, with nothing to notice). pushq.sh's fast checks skip a
// subtest when a push changes none of its prefixes, nor internal/repotest/
// (pq_fast_skip).
const leadShellTable = "scripts/lead/shelltests.txt"

type leadShellEntry struct {
	name     string   // the script's base name, foo_test.sh
	prefixes []string // its inputs, the script itself first
	manual   string   // "NAME manual REASON": not run here, and why
}

func readLeadShellTable(t *testing.T) []leadShellEntry {
	t.Helper()
	f, err := os.Open(filepath.Join(root, leadShellTable))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []leadShellEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fs := strings.Fields(line)
		if len(fs) > 1 && fs[1] == "manual" {
			if len(fs) < 3 {
				t.Errorf("%s: a manual entry in %s needs a reason", fs[0], leadShellTable)
			}
			out = append(out, leadShellEntry{name: fs[0], manual: strings.Join(fs[2:], " ")})
			continue
		}
		out = append(out, leadShellEntry{name: fs[0], prefixes: fs[1:]})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// leadShellArgs: tests that take the checkout as their argument. They run
// scripts/gate.sh on it, which builds the checkout's bin/xo, so they run
// one at a time (leadShellCheckout): two at once removed bin/xo under the
// other's xoclicheck (XOBUILD-STALE, pushq's fast checks, 2026-11-21).
var leadShellArgs = map[string]bool{
	"gate_failfast_test.sh": true,
	"gate_output_test.sh":   true,
}

// leadShellImage: tests that run in a local container; skipped when the
// image is missing (the linux hosts have no Docker).
var leadShellImage = map[string]string{
	"offload-prune_test.sh": "xo-linux-test:arm64-debian-trixie-slim-4",
}

// leadShellName is the subtest name pushq.sh's -skip pattern uses.
func leadShellName(script string) string { return strings.TrimSuffix(script, "_test.sh") }

// TestLeadShell runs each script in shelltests.txt on the Mac, 4 at a time
// (the 55 took 456 s serial on the Mac, 2026-11-21; the slowest 83 s).
func TestLeadShell(t *testing.T) {
	// the lead's tools run on the Mac, and their tests assume it (BSD date
	// -v and sed -i '', plutil, expect, a git identity, origin/main): on
	// linux/arm64 host 3, 2026-11-21, 10 of the 55 failed for those alone.
	// pushq's fast checks, which run them for a changed input, run on the Mac.
	if runtime.GOOS != "darwin" {
		t.Skip("scripts/lead's shell tests run on the Mac (darwin)")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	sem := make(chan struct{}, 4)
	var leadShellCheckout sync.Mutex
	for _, e := range readLeadShellTable(t) {
		t.Run(leadShellName(e.name), func(t *testing.T) {
			t.Parallel()
			if e.manual != "" {
				t.Skipf("run by hand: %s", e.manual)
			}
			if img := leadShellImage[e.name]; img != "" {
				if exec.Command("docker", "image", "inspect", img).Run() != nil {
					t.Skipf("no docker image %s", img)
				}
			}
			if leadShellArgs[e.name] {
				leadShellCheckout.Lock()
				defer leadShellCheckout.Unlock()
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			args := []string{filepath.Join(abs, "scripts", "lead", e.name)}
			if leadShellArgs[e.name] {
				args = append(args, abs)
			}
			cmd := exec.CommandContext(ctx, "bash", args...)
			cmd.Dir = abs
			cmd.Stdin = nil
			// its own process group, stopped whole on the timeout: a killed
			// bash left alone orphans what it started (2026-11-21:
			// offload_test.sh past 10 minutes)
			leadShellGroup(cmd)
			cmd.WaitDelay = 10 * time.Second
			// output to a file, not a pipe: a background child the script
			// leaves would hold a pipe open past its exit (tiersched_test.sh's
			// tier.sh), and what it leaves is stopped with its group below
			of, err := os.CreateTemp(t.TempDir(), "out")
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stdout, cmd.Stderr = of, of
			err = cmd.Run()
			leadShellReap(cmd)
			of.Close()
			out, _ := os.ReadFile(of.Name())
			if err != nil {
				t.Fatalf("%s: %v\n%s", e.name, err, leadShellTail(out))
			}
		})
	}
}

func leadShellTail(out []byte) string {
	ls := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	var keep []string
	for _, l := range ls {
		if !strings.HasPrefix(l, "PASS") && !strings.HasPrefix(l, "ok ") {
			keep = append(keep, l)
		}
	}
	if len(keep) > 40 {
		keep = keep[len(keep)-40:]
	}
	return strings.Join(keep, "\n")
}

// TestLeadShellListed: every scripts/lead/*_test.sh has a caller (named in
// an internal/repotest Go file, or in shelltests.txt), every table entry
// exists, and each entry's prefixes cover the files its script runs,
// followed to the end (leadShellInputs). Reads files only.
func TestLeadShellListed(t *testing.T) {
	table := readLeadShellTable(t)
	listed := map[string]bool{}
	for _, e := range table {
		if listed[e.name] {
			t.Errorf("%s: %s lists it twice", e.name, leadShellTable)
		}
		listed[e.name] = true
	}
	gos, _ := filepath.Glob(filepath.Join(root, "internal", "repotest", "*.go"))
	var goText strings.Builder
	for _, g := range gos {
		if filepath.Base(g) == "leadshell_test.go" {
			continue
		}
		b, err := os.ReadFile(g)
		if err != nil {
			t.Fatal(err)
		}
		goText.Write(b)
	}
	scripts, _ := filepath.Glob(filepath.Join(root, "scripts", "lead", "*_test.sh"))
	if len(scripts) == 0 {
		t.Fatal("no scripts/lead/*_test.sh")
	}
	for _, s := range scripts {
		b := filepath.Base(s)
		other := strings.Contains(goText.String(), `"`+b+`"`)
		switch {
		case other && listed[b]:
			t.Errorf("%s: run by another internal/repotest test and listed in %s; drop one", b, leadShellTable)
		case !other && !listed[b]:
			t.Errorf("%s has no caller: add a line to %s (%s)", b, leadShellTable, strings.Join(leadShellInputs(t, "scripts/lead/"+b), " "))
		}
	}
	for _, e := range table {
		rel := "scripts/lead/" + e.name
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("%s: in %s but no such script", e.name, leadShellTable)
			continue
		}
		if e.manual != "" {
			continue
		}
		if len(e.prefixes) == 0 || e.prefixes[0] != rel {
			t.Errorf("%s: its first input must be %s", e.name, rel)
		}
		var miss []string
		for _, in := range leadShellInputs(t, rel) {
			ok := false
			for _, p := range e.prefixes {
				if strings.HasPrefix(in, p) {
					ok = true
				}
			}
			if !ok {
				miss = append(miss, in)
			}
		}
		if len(miss) > 0 {
			t.Errorf("%s: %s inputs not under its prefixes: %s", e.name, leadShellTable, strings.Join(miss, " "))
		}
	}
}

// a script or program name: foo.sh, bar.py, dir/baz.sh
var leadShellRef = regexp.MustCompile(`[A-Za-z0-9_./-]*[A-Za-z0-9_-]\.(?:sh|py)\b`)

// a file a script runs beside itself: $here/x, $src/x, $(dirname "$0")/x
var leadShellRel = regexp.MustCompile(`(?:\$here|\$\{here\}|\$src|\$\{src\}|\$\(dirname "\$0"\))/([A-Za-z0-9_.-]+)`)

// leadShellCode drops comments (whole comment lines and " #" tails): a
// comment naming a script is not a run of it.
func leadShellCode(b []byte) string {
	var out strings.Builder
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		if i := strings.Index(l, " #"); i >= 0 {
			l = l[:i]
		}
		if i := strings.Index(l, "\t#"); i >= 0 {
			l = l[:i]
		}
		out.WriteString(l)
		out.WriteByte('\n')
	}
	return out.String()
}

// leadShellResolve: a name in a script in dir, as a file beside it or
// under the repository root ("" when neither exists).
func leadShellResolve(dir, m string) string {
	m = strings.TrimPrefix(m, "./")
	for _, c := range []string{dir + "/" + filepath.Base(m), m, strings.TrimPrefix(m, "/")} {
		c = filepath.ToSlash(filepath.Clean(c))
		if strings.HasPrefix(c, "..") {
			continue
		}
		if st, err := os.Stat(filepath.Join(root, c)); err == nil && st.Mode().IsRegular() {
			return c
		}
	}
	return ""
}

// leadShellInputs: the test script, every .sh or .py file it names, and
// what those run beside themselves (leadShellRel, pushq_fast_skip_test.sh's
// rule), followed to the end; the script first, the rest sorted. A test
// stubs most of what its scripts name, so names past the first level count
// only through $here-style references (2026-11-21: following every name
// gave each test about 70 inputs, through pushq.sh and install.sh).
func leadShellInputs(t *testing.T, rel string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{rel: true}
	var todo []string
	add := func(c string) {
		if c != "" && !seen[c] {
			seen[c] = true
			todo = append(todo, c)
		}
	}
	for _, m := range leadShellRef.FindAllString(leadShellCode(b), -1) {
		add(leadShellResolve("scripts/lead", m))
	}
	for len(todo) > 0 {
		f := todo[0]
		todo = todo[1:]
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(f))
		for _, m := range leadShellRel.FindAllStringSubmatch(leadShellCode(b), -1) {
			add(leadShellResolve(dir, m[1]))
		}
	}
	var out []string
	for f := range seen {
		if f != rel {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return append([]string{rel}, out...)
}
