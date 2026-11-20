// Command twincheck builds each bench program and its Go twin, runs both,
// and compares what they print: bench/perf programs that ship a twin.go.txt
// or a go/ directory print the same "<workload> <checksum>" lines, and
// nothing checked that they did until 2026-10-24 (a twin's Cut counted a
// not-found separator where the Xo program's Option gave zero).
//
//	go run ./scripts/twincheck                  # every bench directory with a twin
//	go run ./scripts/twincheck bench/perf/sort  # named ones
//	go run ./scripts/twincheck -time 5          # and time each agreeing case
//
// -time N runs each agreeing case's two programs N times, alternating,
// without twin.txt's env (its smaller iteration counts are for the check),
// and prints `time NAME xo MS go MS ratio R`: the medians of the wall times
// and the median of the per-round Xo / Go ratios, each round one Xo run and
// the Go run after it (decision 0726: a slow VM phase hits both sides of a
// pair). A case whose Go side is under 10 ms runs more rounds, until each
// side has run 1 s in all (at most 1000 rounds). It is a survey's first cut (2026-11-07, the Xo against
// Go re-survey), not an --ab: a claim about one change still takes
// `xo bench --ab` on an offload host.
//
// -xo-b CMD (with -time N) also builds each Xo program with a second xo
// (B; -std-b DIR is its std, as XO_STD) and times A, Go, B, Go per round:
// an A/B of two compilers against the same Go twin (decision 0727's
// scorecard, scripts/scorecard). Each side's ratio pairs its run with the
// Go run after it, and the line `ab NAME go MS a MS b MS ra R rb R rounds
// N` reads the median of the per-round ratios, with -time's round rule
// (decision 0726).
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
	"sort"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/twincheck"
	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	verbose := flag.Bool("v", false, "print every case, not only the mismatches")
	timeout := flag.Duration("timeout", 5*time.Minute, "per build and per run")
	keep := flag.Bool("keep", false, "keep the built binaries and say where")
	times := flag.Int("time", 0, "time each agreeing case's two programs this many times (medians, Xo over Go)")
	xoB := flag.String("xo-b", "", "with -time: a second xo (B), timed against the same Go twin (decision 0727)")
	stdB := flag.String("std-b", "", "with -xo-b: B's std directory (XO_STD for B's builds)")
	flag.Parse()
	if *xoB != "" && *times == 0 {
		fmt.Fprintln(os.Stderr, "twincheck: -xo-b needs -time N")
		os.Exit(2)
	}
	ab = abSide{xo: *xoB, std: *stdB}
	if err := run(*verbose, *timeout, *keep, *times, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "twincheck:", err)
		os.Exit(1)
	}
}

// abSide is -xo-b's compiler: its command and std ("" when not set).
type abSide struct{ xo, std string }

var ab abSide

type checker struct {
	root    string
	xo      []string
	tmp     string
	timeout time.Duration
	verbose bool
	times   int
	builds  map[string]built
}

type built struct {
	path string
	err  error
}

func run(verbose bool, timeout time.Duration, keep bool, times int, dirs []string) error {
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
	c := &checker{root: root, xo: xo, tmp: tmp, timeout: timeout, verbose: verbose, times: times, builds: map[string]built{}}
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
				if c.times > 0 && ab.xo != "" {
					if line, err := c.timeAB(d, cs, name); err != nil {
						fmt.Printf("%s: time: %v\n", name, err)
					} else {
						fmt.Println(line)
					}
				} else if c.times > 0 {
					if line, err := c.timeCase(d, cs, name); err != nil {
						fmt.Printf("%s: time: %v\n", name, err)
					} else {
						fmt.Println(line)
					}
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
	xoBin, err := c.buildXo(d.Path, cs.Xo, d.Backend)
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
func (c *checker) buildXo(rel, prog, backend string) (string, error) {
	return c.buildXoWith(c.xo, "", "xo:", rel, prog, backend)
}

// buildXoWith builds it with the xo command xo (std: its XO_STD, "" for
// the environment's); prefix keeps each compiler's builds apart.
func (c *checker) buildXoWith(xo []string, std, prefix, rel, prog, backend string) (string, error) {
	key := prefix + rel + "/" + prog + ":" + backend
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
	args := append(append([]string{}, xo[1:]...), "build", "-o", out)
	if backend != "" {
		args = append(args, "--backend="+backend)
	}
	args = append(args, dir)
	cmd := exec.CommandContext(ctx, xo[0], args...)
	if std != "" {
		cmd.Env = append(os.Environ(), "XO_STD="+std)
	}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("building xo %s with %s: %v\n%s", dir, xo[0], err, tail(buf.String()))
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
		if err := os.RemoveAll(dir); err != nil {
			return "", err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		src, err := os.ReadFile(target)
		if err != nil {
			return "", err
		}
		gv := filepath.Join(c.root, d.Path, "govendor")
		if st, err := os.Stat(filepath.Join(gv, "vendor")); err == nil && st.IsDir() && len(d.Requires) > 0 {
			// The twin's own vendored modules (govendor/go.mod, go.sum,
			// vendor/ and any local packages): the build needs no module
			// cache or network (2026-11-14: host 1's cache had no
			// golang.org/x/net v0.60.0 for bench/perf/idna).
			if err := os.CopyFS(dir, os.DirFS(gv)); err != nil {
				return "", err
			}
			args = append(args, "-mod=vendor")
		} else {
			mod := "module twincheck.local/twin\n\ngo 1.21\n"
			for _, r := range d.Requires {
				mod += "\nrequire " + r + "\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
				return "", err
			}
			if len(d.Requires) > 0 {
				args = append(args, "-mod=mod")
				env = append(env, "GOPROXY=off", "GOSUMDB=off", "GONOSUMDB=*")
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), src, 0o644); err != nil {
			return "", err
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

// timeCase runs the case's two built programs c.times times each,
// alternating, with the plain environment (no twin.txt env), and returns
// the `time` line: the median wall milliseconds and Xo over Go.
func (c *checker) timeCase(d *twincheck.Dir, cs twincheck.Case, name string) (string, error) {
	dir := filepath.Join(c.root, d.Path)
	xoBin, err := c.buildXo(d.Path, cs.Xo, d.Backend)
	if err != nil {
		return "", err
	}
	goBin, err := c.buildGo(d, cs.Go)
	if err != nil {
		return "", err
	}
	var xs, gs []float64
	for i := 0; i < c.times || moreRounds(xs, gs); i++ {
		for _, side := range []struct {
			bin  string
			args []string
			out  *[]float64
		}{{xoBin, cs.XoArgs, &xs}, {goBin, cs.GoArgs, &gs}} {
			t := time.Now()
			if _, err := c.exec(side.bin, dir, side.args, os.Environ()); err != nil {
				return "", err
			}
			*side.out = append(*side.out, float64(time.Since(t).Microseconds())/1000)
		}
	}
	return fmt.Sprintf("time %s xo %.1f go %.1f ratio %.3f", strings.ReplaceAll(name, " ", "/"), median(xs), median(gs), roundRatio(xs, gs)), nil
}

// timeAB times the case's Xo program built by A (c.xo) and by B (-xo-b)
// against its Go twin: rounds of A, Go, B, Go, each side's ratio taken
// against the Go run after it (decision 0726: a slow phase of the host
// hits both runs of a pair). It checks B's output against Go's first.
func (c *checker) timeAB(d *twincheck.Dir, cs twincheck.Case, name string) (string, error) {
	dir := filepath.Join(c.root, d.Path)
	aBin, err := c.buildXo(d.Path, cs.Xo, d.Backend)
	if err != nil {
		return "", err
	}
	bBin, err := c.buildXoWith(strings.Fields(ab.xo), ab.std, "xob:", d.Path, cs.Xo, d.Backend)
	if err != nil {
		return "", err
	}
	goBin, err := c.buildGo(d, cs.Go)
	if err != nil {
		return "", err
	}
	env := append(os.Environ(), d.Env...)
	bOut, err := c.exec(bBin, dir, cs.XoArgs, env)
	if err != nil {
		return "", fmt.Errorf("B's Xo program: %v", err)
	}
	goOut, err := c.exec(goBin, dir, cs.GoArgs, env)
	if err != nil {
		return "", fmt.Errorf("the Go twin: %v", err)
	}
	if diff := twincheck.Compare(bOut, goOut, d.Fields); diff != "" {
		return "", fmt.Errorf("B's Xo program differs from the Go twin: %s", diff)
	}
	run := func(bin string, args []string) (float64, error) {
		t := time.Now()
		if _, err := c.exec(bin, dir, args, os.Environ()); err != nil {
			return 0, err
		}
		return float64(time.Since(t).Microseconds()) / 1000, nil
	}
	var as, bs, ga, gb []float64
	for i := 0; i < c.times || moreRounds(as, ga) || moreRounds(bs, gb); i++ {
		var r [4]float64
		for j, side := range []struct {
			bin  string
			args []string
		}{{aBin, cs.XoArgs}, {goBin, cs.GoArgs}, {bBin, cs.XoArgs}, {goBin, cs.GoArgs}} {
			if r[j], err = run(side.bin, side.args); err != nil {
				return "", err
			}
		}
		as, ga, bs, gb = append(as, r[0]), append(ga, r[1]), append(bs, r[2]), append(gb, r[3])
	}
	return fmt.Sprintf("ab %s go %.1f a %.1f b %.1f ra %.3f rb %.3f rounds %d", strings.ReplaceAll(name, " ", "/"), median(append(append([]float64{}, ga...), gb...)), median(as), median(bs), roundRatio(as, ga), roundRatio(bs, gb), len(as)), nil
}

// moreRounds reports whether a case past its -time rounds runs another:
// its Go median is under 10 ms and a side has run under 1 s in all
// (decision 0726), up to 1000 rounds.
func moreRounds(xs, gs []float64) bool {
	if len(gs) == 0 || len(gs) >= 1000 || median(gs) >= 10 {
		return false
	}
	return sum(xs) < 1000 || sum(gs) < 1000
}

func sum(v []float64) float64 {
	t := 0.0
	for _, x := range v {
		t += x
	}
	return t
}

// roundRatio is the median of the per-round ratios xs[i] / gs[i]
// (decision 0726).
func roundRatio(xs, gs []float64) float64 {
	r := make([]float64, len(xs))
	for i := range xs {
		r[i] = xs[i] / gs[i]
	}
	return median(r)
}

// median is the middle value, the mean of the two middle ones for an even
// count.
func median(v []float64) float64 {
	s := append([]float64{}, v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// tail is the last lines of a failed command's output.
func tail(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.Join(lines, "\n")
}
