// Command twincheck builds each bench program and its Go twin, runs both,
// and compares what they print: bench/perf programs that ship a twin.go.txt
// or a go/ directory print the same "<workload> <checksum>" lines, and
// nothing checked that they did until 2026-10-24 (a twin's Cut counted a
// not-found separator where the Xo program's Option gave zero).
//
//	go run ./scripts/twincheck                  # every bench directory with a twin
//	go run ./scripts/twincheck bench/perf/sort  # named ones
//
// A bench directory's twin.txt says how to run it (internal/twincheck has
// the format: cases, arguments, compare fields, env, skip). Exit 1 when a
// case's output differs or a program does not build.
//
// It builds and runs programs, so an agent runs it through heavy.sh.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/twincheck"
	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	verbose := flag.Bool("v", false, "print every case, not only the mismatches")
	timeout := flag.Duration("timeout", 5*time.Minute, "per build and per run")
	keep := flag.Bool("keep", false, "keep the built binaries and say where")
	flag.Parse()
	if err := run(*verbose, *timeout, *keep, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "twincheck:", err)
		os.Exit(1)
	}
}

type checker struct {
	root    string
	xo      []string
	tmp     string
	timeout time.Duration
	verbose bool
	builds  map[string]built
}

type built struct {
	path string
	err  error
}

func run(verbose bool, timeout time.Duration, keep bool, dirs []string) error {
	root, err := xotool.Root()
	if err != nil {
		return err
	}
	xo, err := xotool.Command()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "twincheck-")
	if err != nil {
		return err
	}
	if keep {
		fmt.Println("twincheck: keeping", tmp)
	} else {
		defer os.RemoveAll(tmp)
	}
	c := &checker{root: root, xo: xo, tmp: tmp, timeout: timeout, verbose: verbose, builds: map[string]built{}}
	if len(dirs) == 0 {
		if dirs, err = twincheck.Discover(root); err != nil {
			return err
		}
	}
	var skipped, cases, bad int
	for _, rel := range dirs {
		d, err := twincheck.Load(root, rel)
		if err != nil {
			fmt.Printf("%s: %v\n", rel, err)
			bad++
			continue
		}
		if d.Skip != "" {
			fmt.Printf("%s: skipped (%s)\n", rel, d.Skip)
			skipped++
			continue
		}
		for _, cs := range d.Cases {
			cases++
			name := rel
			if n := cs.Name(); n != "" {
				name += " " + n
			}
			diff, err := c.check(d, cs)
			switch {
			case err != nil:
				fmt.Printf("%s: FAIL %v\n", name, err)
				bad++
			case diff != "":
				fmt.Printf("%s: MISMATCH %s\n", name, diff)
				bad++
			default:
				if verbose {
					fmt.Printf("%s: ok\n", name)
				}
			}
		}
	}
	fmt.Printf("twincheck: %d cases, %d mismatches or failures, %d directories skipped\n", cases, bad, skipped)
	if bad > 0 {
		return fmt.Errorf("%d of %d cases disagree", bad, cases)
	}
	return nil
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func (c *checker) bin(name string) string {
	return filepath.Join(c.tmp, unsafe.ReplaceAllString(name, "_"))
}

// check builds both sides, runs them with the case's arguments, and returns
// the difference ("" when they agree).
func (c *checker) check(d *twincheck.Dir, cs twincheck.Case) (string, error) {
	dir := filepath.Join(c.root, d.Path)
	xoBin, err := c.buildXo(d.Path, cs.Xo)
	if err != nil {
		return "", err
	}
	goBin, err := c.buildGo(d, cs.Go)
	if err != nil {
		return "", err
	}
	env := append(os.Environ(), d.Env...)
	xoOut, err := c.exec(xoBin, dir, cs.XoArgs, env)
	if err != nil {
		return "", fmt.Errorf("the Xo program: %v", err)
	}
	goOut, err := c.exec(goBin, dir, cs.GoArgs, env)
	if err != nil {
		return "", fmt.Errorf("the Go twin: %v", err)
	}
	return twincheck.Compare(xoOut, goOut, d.Fields), nil
}

// buildXo builds the Xo program (a file, or a program directory), once per
// program.
func (c *checker) buildXo(rel, prog string) (string, error) {
	key := "xo:" + rel + "/" + prog
	if b, ok := c.builds[key]; ok {
		return b.path, b.err
	}
	target := filepath.Join(c.root, rel, prog)
	dir := target
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		dir = filepath.Dir(target)
	}
	out := c.bin(key)
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	args := append(append([]string{}, c.xo[1:]...), "build", "-o", out, dir)
	cmd := exec.CommandContext(ctx, c.xo[0], args...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("building xo %s: %v\n%s", dir, err, tail(buf.String()))
	}
	c.builds[key] = built{out, err}
	return out, err
}

// buildGo builds the Go twin: a module directory, or a single .go (or
// .go.txt) file copied into a module of its own. That module carries the
// directory's `require` lines and builds offline from the module cache, so
// a twin that imports golang.org/x/crypto needs no network.
func (c *checker) buildGo(d *twincheck.Dir, prog string) (string, error) {
	key := "go:" + d.Path + "/" + prog
	if b, ok := c.builds[key]; ok {
		return b.path, b.err
	}
	target := filepath.Join(c.root, d.Path, prog)
	out := c.bin(key)
	var dir string
	args := []string{"build", "-o", out}
	env := os.Environ()
	if st, err := os.Stat(target); err == nil && st.IsDir() {
		dir = target
	} else {
		dir = c.bin(key + "-module")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		src, err := os.ReadFile(target)
		if err != nil {
			return "", err
		}
		mod := "module twincheck.local/twin\n\ngo 1.21\n"
		for _, r := range d.Requires {
			mod += "\nrequire " + r + "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), src, 0o644); err != nil {
			return "", err
		}
		if len(d.Requires) > 0 {
			args = append(args, "-mod=mod")
			env = append(env, "GOPROXY=off", "GOSUMDB=off", "GONOSUMDB=*")
		}
	}
	args = append(args, ".")
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("building go %s: %v\n%s", target, err, tail(buf.String()))
	}
	c.builds[key] = built{out, err}
	return out, err
}

// exec runs one binary and returns its stdout; a timeout or a nonzero exit
// is an error that names the side.
func (c *checker) exec(bin, dir string, args, env []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %v\n%s", filepath.Base(bin), strings.Join(args, " "), err, tail(errOut.String()))
	}
	return out.String(), nil
}

// tail is the last lines of a failed command's output.
func tail(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.Join(lines, "\n")
}
