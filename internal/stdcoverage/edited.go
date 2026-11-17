package stdcoverage

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// The edited-module check (owner, 2026-11-17, task #259): TestStdAPICovered
// on the std modules a change edits only. The declarations of each edited
// module need a resolved call from a program whose build reads one of the
// module's files (`xo build --list-inputs`: the reverse closure), or an
// allow.txt line; a program the change edits or deletes adds the modules
// its version at the base used, which may have lost their last caller.
// The full sweep of every program stays in the tier as the backstop.

// moduleOfStdFile is the API module a std file belongs to, "" for one
// that declares no API (llrt, std/rt, tests, docs): std/M.xo and its
// implementation std/native/M.xo and std/native/M_native.xo are M,
// std/native/prelude.xo is prelude.
func moduleOfStdFile(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasSuffix(p, ".xo") || strings.HasSuffix(p, "_test.xo") {
		return ""
	}
	if p == "std/native/prelude.xo" {
		return "prelude"
	}
	if rest, ok := strings.CutPrefix(p, "std/native/"); ok && !strings.Contains(rest, "/") {
		return strings.TrimSuffix(strings.TrimSuffix(rest, ".xo"), "_native")
	}
	if rest, ok := strings.CutPrefix(p, "std/"); ok && !strings.Contains(rest, "/") {
		return strings.TrimSuffix(rest, ".xo")
	}
	return ""
}

var useStd = regexp.MustCompile(`(?m)^\s*use\s+std/([a-z0-9_]+)`)

func git(root string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = root
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// EditedModules lists the std API modules a change since base edits, and
// the modules the change's edited or deleted programs used at base (the
// `use std/M` lines of their old files; every program uses the prelude).
func EditedModules(root, base string, programDirs []string) (edited, lostCallers []string, err error) {
	out, err := git(root, "diff", "--name-status", base, "--")
	if err != nil {
		return nil, nil, err
	}
	isProg := map[string]bool{}
	for _, d := range programDirs {
		isProg[filepath.ToSlash(d)] = true
	}
	ed := map[string]bool{}
	lost := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		path := f[len(f)-1]
		if m := moduleOfStdFile(path); m != "" {
			ed[m] = true
		}
		// a program's file edited or deleted (status M or D; a rename's
		// old name is f[1]): the modules its base version used
		if !strings.HasSuffix(f[1], ".xo") || !(f[0] == "M" || f[0] == "D" || strings.HasPrefix(f[0], "R")) {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(f[1]))
		for d := dir; d != "." && d != "/"; d = filepath.ToSlash(filepath.Dir(d)) {
			if isProg[d] {
				if old, err := git(root, "show", base+":"+f[1]); err == nil {
					for _, m := range useStd.FindAllStringSubmatch(old, -1) {
						lost[m[1]] = true
					}
				}
				break
			}
		}
	}
	for m := range ed {
		edited = append(edited, m)
	}
	for m := range lost {
		if !ed[m] {
			lostCallers = append(lostCallers, m)
		}
	}
	sort.Strings(edited)
	sort.Strings(lostCallers)
	return edited, lostCallers, nil
}

// moduleFiles are the files whose presence in a program's inputs means
// the program reaches module m.
func moduleFiles(m string) []string {
	if m == "prelude" {
		return []string{"std/native/prelude.xo"}
	}
	return []string{"std/" + m + ".xo", "std/native/" + m + ".xo", "std/native/" + m + "_native.xo"}
}

// ProgramsReaching lists the program directories whose `xo build
// --list-inputs` includes one of files (relative to root), jobs at once.
func ProgramsReaching(root, xo string, dirs, files []string, jobs int) ([]string, error) {
	want := map[string]bool{}
	for _, f := range files {
		want[f] = true
	}
	hit := make([]bool, len(dirs))
	next := make(chan int)
	var wg sync.WaitGroup
	if jobs < 1 {
		jobs = 1
	}
	for w := 0; w < jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				c := exec.Command(xo, "build", "--list-inputs", dirs[i])
				c.Dir = root
				out, err := c.Output()
				if err != nil {
					// a program whose inputs cannot be listed is swept:
					// it errs toward checking
					hit[i] = true
					continue
				}
				for _, l := range bytes.Split(out, []byte("\n")) {
					p := strings.TrimPrefix(filepath.ToSlash(string(bytes.TrimSpace(l))), filepath.ToSlash(root)+"/")
					if want[p] {
						hit[i] = true
						break
					}
				}
			}
		}()
	}
	for i := range dirs {
		next <- i
	}
	close(next)
	wg.Wait()
	var out []string
	for i, d := range dirs {
		if hit[i] {
			out = append(out, d)
		}
	}
	return out, nil
}

// CheckEdited is TestStdAPICovered on the modules a change since base
// edits (and those its edited programs used): every declaration of those
// modules needs a resolved call from a program reaching them, or an
// allow.txt line. It returns the modules checked, the programs swept, the
// open declarations and the programs the sweep could not check.
func CheckEdited(root, xo, work, base string, jobs int) (modules, swept []string, open []Entry, failed []string, err error) {
	dirs, err := ProgramDirs(root)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	edited, lost, err := EditedModules(root, base, dirs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	modules = append(append([]string{}, edited...), lost...)
	sort.Strings(modules)
	if len(modules) == 0 {
		return nil, nil, nil, nil, nil
	}
	var files []string
	for _, m := range modules {
		for _, f := range moduleFiles(m) {
			if _, err := os.Stat(filepath.Join(root, f)); err == nil {
				files = append(files, f)
			}
		}
	}
	swept, err = ProgramsReaching(root, xo, dirs, files, jobs)
	if err != nil {
		return modules, nil, nil, nil, err
	}
	var edges []Edge
	if len(swept) > 0 {
		edges, failed, err = SweepDirs(root, xo, work, jobs, swept)
		if err != nil {
			return modules, swept, nil, failed, err
		}
	}
	uncovered, _, _, err := UncoveredResolved(root, edges)
	if err != nil {
		return modules, swept, nil, failed, err
	}
	check := map[string]bool{}
	for _, m := range modules {
		check[m] = true
	}
	var mine []Entry
	for _, e := range uncovered {
		if check[e.Module] {
			mine = append(mine, e)
		}
	}
	rules, err := Allow()
	if err != nil {
		return modules, swept, nil, failed, err
	}
	open, _ = Unallowed(mine, rules)
	return modules, swept, open, failed, nil
}
