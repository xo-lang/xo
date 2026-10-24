// Package twincheck lets a benchmark compare its Xo program against its Go
// twin: bench/perf programs that ship a twin.go.txt or a go/ directory
// print the same "<workload> <checksum>" lines, and nothing checked that
// they did until 2026-10-24 (a twin's Cut counted a not-found separator
// where the Xo program's Option gave zero, and the drift went unnoticed in
// the results).
//
// Discovery finds each bench directory with both sides; a twin.txt in the
// directory says how to run it:
//
//	# a comment
//	skip REASON                 # this directory is not comparable here
//	compare fields N            # compare the first N fields of each line
//	env KEY=VALUE               # for both sides (a smaller iteration count)
//	require MODULE VERSION      # what a single-file Go twin imports from outside the standard library (built from the module cache, offline)
//	XOPROG GOPROG [ARGS] [-- GOARGS]
//
// A case line names the Xo program (a .xo file or a program directory) and
// the Go program (a .go or .go.txt file, or a module directory), both
// relative to the bench directory, then the arguments to pass: ARGS go to
// both sides, and a "-- GOARGS" after them gives the Go side its own (a
// program whose Go twin takes a mode argument). "-" in either program slot
// means the directory's default (main.xo or xo/, and twin.go.txt or go/).
// Without a twin.txt the directory runs as one case with no arguments.
package twincheck

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Case is one program pair to run.
type Case struct {
	Xo, Go         string // program paths relative to the bench directory
	XoArgs, GoArgs []string
}

// Dir is one bench directory to check.
type Dir struct {
	Path     string // relative to the repository root, slash separated
	Cases    []Case
	Skip     string   // a declared reason not to run this directory
	Fields   int      // compare the first Fields fields of each line; 0 for the whole line
	Env      []string // KEY=VALUE for both sides
	Requires []string // "MODULE VERSION": what a single-file Go twin imports from outside the standard library
}

// ConfigName is the file, in a bench directory, that says how to run it.
const ConfigName = "twin.txt"

// DefaultXo and DefaultGo name the directory's default programs.
const (
	DefaultXo = "-"
	DefaultGo = "-"
)

// Discover lists the bench directories under root that have a Go twin: one
// level for bench/perf/NAME and two for bench/perf/GROUP/NAME (twins/hello).
func Discover(root string) ([]string, error) {
	perf := filepath.Join(root, "bench", "perf")
	entries, err := os.ReadDir(perf)
	if err != nil {
		return nil, err
	}
	var out []string
	add := func(rel string) {
		if HasTwin(filepath.Join(root, rel)) {
			out = append(out, rel)
		}
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rel := "bench/perf/" + e.Name()
		add(rel)
		subs, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		for _, s := range subs {
			if s.IsDir() {
				add(rel + "/" + s.Name())
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// HasTwin says whether dir holds both an Xo bench program and a Go twin.
func HasTwin(dir string) bool { return hasXo(dir) && hasGo(dir) }

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func hasXo(dir string) bool {
	return exists(filepath.Join(dir, "main.xo")) || isDir(filepath.Join(dir, "xo"))
}

func hasGo(dir string) bool {
	return exists(filepath.Join(dir, "twin.go.txt")) || isDir(filepath.Join(dir, "go"))
}

// defaultXo is the Xo program a "-" names: main.xo when the bench directory
// holds one, else the xo/ module directory.
func defaultXo(dir string) (string, error) {
	if exists(filepath.Join(dir, "main.xo")) {
		return "main.xo", nil
	}
	if isDir(filepath.Join(dir, "xo")) {
		return "xo", nil
	}
	return "", fmt.Errorf("%s: no main.xo and no xo/ to run", dir)
}

// defaultGo is the Go twin a "-" names: twin.go.txt when present, else go/.
func defaultGo(dir string) (string, error) {
	if exists(filepath.Join(dir, "twin.go.txt")) {
		return "twin.go.txt", nil
	}
	if isDir(filepath.Join(dir, "go")) {
		return "go", nil
	}
	return "", fmt.Errorf("%s: no twin.go.txt and no go/ to run", dir)
}

// Load reads one bench directory's twin.txt, or its default single case.
func Load(root, rel string) (*Dir, error) {
	d := &Dir{Path: rel}
	dir := filepath.Join(root, rel)
	data, err := os.ReadFile(filepath.Join(dir, ConfigName))
	switch {
	case os.IsNotExist(err):
		xo, xerr := defaultXo(dir)
		if xerr != nil {
			return nil, xerr
		}
		g, gerr := defaultGo(dir)
		if gerr != nil {
			return nil, gerr
		}
		d.Cases = []Case{{Xo: xo, Go: g}}
		return d, nil
	case err != nil:
		return nil, err
	}
	if err := parse(d, dir, string(data)); err != nil {
		return nil, fmt.Errorf("%s/%s: %v", rel, ConfigName, err)
	}
	if d.Skip == "" && len(d.Cases) == 0 {
		return nil, fmt.Errorf("%s/%s: no cases", rel, ConfigName)
	}
	return d, nil
}

func parse(d *Dir, dir, data string) error {
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if k := strings.IndexByte(line, '#'); k >= 0 {
			line = strings.TrimSpace(line[:k])
		}
		if line == "" {
			continue
		}
		name, rest, _ := strings.Cut(line, " ")
		name = strings.TrimSuffix(name, ":") // "skip:" reads as "skip"
		switch name {
		case "skip":
			if strings.TrimSpace(rest) == "" {
				return fmt.Errorf("line %d: skip needs a reason", i+1)
			}
			d.Skip = strings.TrimSpace(rest)
		case "compare":
			what, n, ok := strings.Cut(strings.TrimSpace(rest), " ")
			if !ok || what != "fields" {
				return fmt.Errorf("line %d: compare takes \"fields N\"", i+1)
			}
			v, err := strconv.Atoi(strings.TrimSpace(n))
			if err != nil || v < 1 {
				return fmt.Errorf("line %d: compare fields needs a count of 1 or more", i+1)
			}
			d.Fields = v
		case "env":
			kv := strings.TrimSpace(rest)
			k, _, ok := strings.Cut(kv, "=")
			if !ok || k == "" {
				return fmt.Errorf("line %d: env takes KEY=VALUE", i+1)
			}
			d.Env = append(d.Env, kv)
		case "require":
			f := strings.Fields(rest)
			if len(f) != 2 {
				return fmt.Errorf("line %d: require takes MODULE VERSION", i+1)
			}
			d.Requires = append(d.Requires, f[0]+" "+f[1])
		default:
			c, err := parseCase(line)
			if err != nil {
				return fmt.Errorf("line %d: %v", i+1, err)
			}
			d.Cases = append(d.Cases, c)
		}
	}
	if err := resolve(d, dir); err != nil {
		return err
	}
	return nil
}

// parseCase reads "XOPROG GOPROG [ARGS] [-- GOARGS]": ARGS go to both sides
// unless a "-- GOARGS" gives the Go side its own.
func parseCase(line string) (Case, error) {
	var c Case
	left, right, split := strings.Cut(line, "--")
	l := strings.Fields(left)
	if len(l) < 2 {
		return c, fmt.Errorf("a case is \"XOPROG GOPROG [ARGS]\", with - meaning the default (got %q)", line)
	}
	c.Xo, c.Go = strings.TrimSuffix(l[0], "/"), strings.TrimSuffix(l[1], "/")
	c.XoArgs = append([]string{}, l[2:]...)
	if split {
		c.GoArgs = strings.Fields(right)
	} else {
		c.GoArgs = append([]string{}, c.XoArgs...)
	}
	return c, nil
}

// resolve replaces "-" with the directory's default programs.
func resolve(d *Dir, dir string) error {
	for i := range d.Cases {
		c := &d.Cases[i]
		if c.Xo == DefaultXo {
			xo, err := defaultXo(dir)
			if err != nil {
				return err
			}
			c.Xo = xo
		}
		if c.Go == DefaultGo {
			g, err := defaultGo(dir)
			if err != nil {
				return err
			}
			c.Go = g
		}
	}
	return nil
}

// Compare returns "" when the two programs' output agrees, else a report
// naming the first line that differs. With fields > 0 only the first
// fields fields of each line are compared (a program that prints its own
// timing in the line).
func Compare(xoOut, goOut string, fields int) string {
	xl := lines(xoOut)
	gl := lines(goOut)
	if len(xl) != len(gl) {
		return fmt.Sprintf("%d lines from the Xo program, %d from the Go twin", len(xl), len(gl))
	}
	for i := range xl {
		a, b := xl[i], gl[i]
		if fields > 0 {
			a, b = firstFields(a, fields), firstFields(b, fields)
		}
		if a != b {
			return fmt.Sprintf("line %d differs:\n  xo: %s\n  go: %s", i+1, xl[i], gl[i])
		}
	}
	return ""
}

func lines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func firstFields(line string, n int) string {
	f := strings.Fields(line)
	if len(f) > n {
		f = f[:n]
	}
	return strings.Join(f, " ")
}

// Name is a case's display name: its arguments, and both sides only when
// the two differ.
func (c Case) Name() string {
	x, g := strings.Join(c.XoArgs, " "), strings.Join(c.GoArgs, " ")
	switch {
	case x == "" && g == "":
		return ""
	case x == "":
		return g
	case g == "" || g == x:
		return x
	}
	return x + " / " + g
}
