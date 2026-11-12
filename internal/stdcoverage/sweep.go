package stdcoverage

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Jobs is how many `xo dev edges` the sweep runs at once: XO_STDCOV_JOBS,
// else 2. Each check holds a program and std in memory (up to about 2 GB),
// and runtime.NumCPU() of them at once peaked at 32 GB on a 24 GB Mac
// beside other gates (2026-11-11), so the default stays small.
func Jobs() int {
	if v := os.Getenv("XO_STDCOV_JOBS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 2
}

// Sweep runs the resolved coverage sweep: `xo dev edges --impls --own` of
// every program directory (ProgramDirs), then `xo dev edges --impls --std`
// once for each bindings value the programs name, and returns the edges
// for UncoveredResolved. Each program's own edges are its sites alone (a
// check without std's bodies, about 0.3 s), and the std graph is the same
// for every program with the same bindings, so it is read once a value
// instead of once a program (the full graph with each of 1,381 programs
// was 47 GB). A program that stops on a missing `use go` or `use c`
// binding is checked with `xo check`, which binds what it lacks, and
// swept again after the others; one that still fails is returned in failed, with the
// first line of what xo said. xo is the xo to run, jobs how many run at
// once, and work an empty directory for the edge files.
func Sweep(root, xo, work string, jobs int) (edges []Edge, failed []string, err error) {
	dirs, err := ProgramDirs(root)
	if err != nil {
		return nil, nil, err
	}
	// XO_STDCOV_ONLY (a regular expression) sweeps only the programs it
	// matches, to look at a few; the coverage it gives is not std's
	if only := os.Getenv("XO_STDCOV_ONLY"); only != "" {
		re, err := regexp.Compile(only)
		if err != nil {
			return nil, nil, fmt.Errorf("XO_STDCOV_ONLY: %v", err)
		}
		var kept []string
		for _, d := range dirs {
			if re.MatchString(filepath.ToSlash(d)) {
				kept = append(kept, d)
			}
		}
		dirs = kept
	}
	if jobs < 1 {
		jobs = 1
	}
	type result struct {
		path     string
		bindings string
		fail     string
	}
	results := make([]result, len(dirs))
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out := filepath.Join(work, fmt.Sprintf("own-%d.tsv", i))
				msg, ok := runEdges(root, xo, dirs[i], out, "--own")
				if !ok {
					results[i] = result{fail: msg}
					continue
				}
				results[i] = result{path: out, bindings: bindingsOf(out)}
			}
		}()
	}
	for i := range dirs {
		next <- i
	}
	close(next)
	wg.Wait()
	// A program that stopped on a missing binding: xo check binds it, one
	// program at a time (into $XO_CBIND_DIR or $XO_GOBIND_DIR when set, as
	// a test's environment sets them, which xo dev edges reads too), and
	// the program is swept again.
	for i, r := range results {
		if r.fail == "" || !strings.Contains(r.fail, "binding") {
			continue
		}
		c := exec.Command(xo, "check", dirs[i])
		c.Dir = root
		// a program may fail its check on purpose; the retry says whether
		// the bindings were written
		checked, _ := c.CombinedOutput()
		out := filepath.Join(work, fmt.Sprintf("own-%d.tsv", i))
		if msg, ok := runEdges(root, xo, dirs[i], out, "--own"); ok {
			results[i] = result{path: out, bindings: bindingsOf(out)}
		} else {
			results[i] = result{fail: firstLine(msg) + " (after xo check: " + firstLine(string(checked)) + ")"}
		}
	}
	for i, r := range results {
		if r.fail != "" && !strings.HasPrefix(r.fail, dirs[i]+": ") {
			results[i].fail = dirs[i] + ": " + firstLine(r.fail)
		}
	}
	var paths []string
	first := map[string]int{} // a bindings value: the first program that has it
	for i, r := range results {
		if r.fail != "" {
			failed = append(failed, r.fail)
			continue
		}
		paths = append(paths, r.path)
		if _, ok := first[r.bindings]; !ok {
			first[r.bindings] = i
		}
	}
	values := make([]string, 0, len(first))
	for b := range first {
		values = append(values, b)
	}
	sort.Strings(values)
	for _, b := range values {
		out := filepath.Join(work, "std-"+strings.ReplaceAll(b, ",", "-")+".tsv")
		if msg, ok := runEdges(root, xo, dirs[first[b]], out, "--std"); !ok {
			return nil, failed, fmt.Errorf("the std graph for bindings %s (from %s): %s", b, dirs[first[b]], firstLine(msg))
		}
		paths = append(paths, out)
	}
	edges, err = ReadEdges(paths)
	return edges, failed, err
}

// runEdges runs `xo dev edges --impls PART --out=OUT DIR` in root and says
// whether it succeeded, with what it printed.
func runEdges(root, xo, dir, out, part string) (string, bool) {
	c := exec.Command(xo, "dev", "edges", "--impls", part, "--out="+out, dir)
	c.Dir = root
	var b bytes.Buffer
	c.Stdout = &b
	c.Stderr = &b
	err := c.Run()
	return b.String(), err == nil
}

// bindingsOf reads the `# bindings B` line `xo dev edges --own` writes
// first ("none" when it is missing).
func bindingsOf(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "none"
	}
	line, _, _ := strings.Cut(string(data), "\n")
	if b, ok := strings.CutPrefix(line, "# bindings "); ok {
		return strings.TrimSpace(b)
	}
	return "none"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// Unallowed checks entries against the allowlist rules: the entries no rule
// matches, and the rules that match no entry (a stale line).
func Unallowed(entries []Entry, rules []Rule) (open []Entry, unused []Rule) {
	used := make([]bool, len(rules))
	for _, e := range entries {
		allowed := false
		for i, r := range rules {
			if r.Match(e) {
				used[i] = true
				allowed = true
			}
		}
		if !allowed {
			open = append(open, e)
		}
	}
	for i, r := range rules {
		if !used[i] {
			unused = append(unused, r)
		}
	}
	return open, unused
}
