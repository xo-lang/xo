// Command stdproven runs the testing evidence of decision 0616 for one std
// module and writes it to std/evidence/MODULE/KIND.md:
//
//	go run ./scripts/stdproven hash MODULE
//	go run ./scripts/stdproven fuzz [-cases N] [-hours H] [-batch N] [-backends B,...] MODULE
//	go run ./scripts/stdproven timing [-n N] [-backends B,...] -pairs A:B,... MODULE NAME
//	go run ./scripts/stdproven race [-backend B] [-host LABEL] MODULE
//
// race (decision 0616 section 2) runs the module's own tests
// (std/native/MODULE_test.xo) and every corpus directory of
// internal/stdtest/testdata/corpus named MODULE or starting with it, each
// with xo test --race, and writes std/evidence/MODULE/race.md when none
// fails or reports a data race. (The stress program under --race joins it
// when the module has one.)
//
// timing (decision 0616 checklist item 3) builds the dudect program
// std/evidence/MODULE/timing/NAME/ on each backend and runs it once per
// pair of input classes with -n measurements (default 1,000,000); the
// program prints a last line "A vs B: N measurements, max |t| T, pass".
// It writes std/evidence/MODULE/timing-NAME.md when every |t| is below
// 4.5, and fails with the program's output otherwise.
//
// hash prints the module's source hash (decision 0616 section 3) and the
// files it covers. fuzz runs every fuzz harness that names the module with
// fresh seeds, a batch of cases a seed, until each backend has run -cases
// cases (default 1,000,000) or -hours have passed (default 8), whichever
// comes first, and stops at the first failing batch. A passing run writes
// std/evidence/MODULE/fuzz.md (its header first, then the batches); a
// failing one writes nothing there and prints the seed and the failing
// output, for a fix with a regression case. A later run starts past the
// seeds the module's fuzz.md records.
//
// It runs where it is started (an offload host for the long runs: decision
// 0616 section 4; scripts/lead/stdproven-offload.sh sends it to the
// linux/arm64 pass/fail host outside the lane, as soaks run). It uses the
// checkout's xo as the Go tests do (internal/xotool).
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// harness is one fuzz test of internal/stdtest: its test name and the
// prefix of its XO_<NAME>_SEED, _N and _BACKENDS variables.
type harness struct {
	test, env string
}

// harnesses lists, per module, the fuzz tests that name it. A module with
// none has no fuzz evidence to run (decision 0461: a parser of untrusted
// input gets a harness first, its Solid requirement).
var harnesses = map[string][]harness{
	"json": {{"TestJsonFuzz", "XO_JSONFUZZ"}},
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "hash":
		if len(os.Args) != 3 {
			usage()
		}
		var h string
		var files []string
		h, files, err = sourceHash(os.Args[2])
		if err == nil {
			fmt.Println(h)
			for _, f := range files {
				fmt.Println("  " + f)
			}
		}
	case "fuzz":
		err = fuzz(os.Args[2:])
	case "timing":
		err = timing(os.Args[2:])
	case "race":
		err = race(os.Args[2:])
	case "stress":
		err = stress(os.Args[2:])
	case "perf":
		err = perf(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdproven:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: stdproven hash MODULE | stdproven fuzz [-cases N] [-hours H] [-batch N] [-backends B,...] MODULE | stdproven timing [-n N] [-backends B,...] -pairs A:B,... MODULE NAME | stdproven race [-backend B] [-stress-minutes M] MODULE | stdproven stress [-minutes M] [-tasks N] [-backends B,...] MODULE | stdproven perf [-rounds N] MODULE")
	os.Exit(2)
}

var useLine = regexp.MustCompile(`^use\s+([a-z0-9_/]+)`)

// sourceHash is the SHA-256 over the paths and contents, sorted by path, of
// the module's std/MODULE.xo and std/native/MODULE.xo and of every std file
// they reach through use lines (std/X, native/X, native/llrt/X), tests
// left out.
func sourceHash(module string) (string, []string, error) {
	seen := map[string]bool{}
	var queue []string
	add := func(p string) {
		if seen[p] || strings.HasSuffix(p, "_test.xo") {
			return
		}
		if _, err := os.Stat(p); err != nil {
			return
		}
		seen[p] = true
		queue = append(queue, p)
	}
	add(filepath.Join("std", module+".xo"))
	add(filepath.Join("std", "native", module+".xo"))
	if len(queue) == 0 {
		return "", nil, fmt.Errorf("no std/%s.xo or std/native/%s.xo (run from the repository root)", module, module)
	}
	for i := 0; i < len(queue); i++ {
		f, err := os.Open(queue[i])
		if err != nil {
			return "", nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			m := useLine.FindStringSubmatch(sc.Text())
			if m == nil {
				continue
			}
			u := m[1]
			switch {
			case strings.HasPrefix(u, "std/"):
				add(filepath.Join("std", u[4:]+".xo"))
				add(filepath.Join("std", "native", u[4:]+".xo"))
			case strings.HasPrefix(u, "native/"):
				add(filepath.Join("std", u+".xo"))
			}
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return "", nil, err
		}
	}
	files := make([]string, 0, len(seen))
	for p := range seen {
		files = append(files, p)
	}
	sort.Strings(files)
	h := sha256.New()
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", nil, err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(p), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), files, nil
}

// header is an evidence file's parsed first lines (key: value).
type header map[string]string

func readHeader(path string) (header, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	h := header{}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == "" {
			break
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			break
		}
		h[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return h, nil
}

// fuzz runs the long fuzz of decision 0616 section 2 for one module.
func fuzz(args []string) error {
	fs := flag.NewFlagSet("fuzz", flag.ExitOnError)
	cases := fs.Int("cases", 1000000, "cases per backend to reach")
	hours := fs.Float64("hours", 8, "time limit")
	batch := fs.Int("batch", 5000, "cases a seed")
	backends := fs.String("backends", "arm64,llvm", "backends, comma separated")
	hostLabel := fs.String("host", "", "the host's name in the evidence (no network names: e.g. \"linux/arm64 host 2\")")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	hs := harnesses[module]
	if len(hs) == 0 {
		return fmt.Errorf("no fuzz harness names %s (scripts/stdproven harnesses)", module)
	}
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", "compiler", "internal/stdtest"); dirty != "" {
		return errors.New("std, compiler or internal/stdtest has uncommitted changes: evidence names a commit")
	}
	out := filepath.Join("std", "evidence", module, "fuzz.md")
	first := 2 // seed 1 is each harness's committed corpus
	if h, err := readHeader(out); err == nil {
		if _, last, ok := strings.Cut(h["seeds"], " to "); ok {
			if n, err := strconv.Atoi(last); err == nil {
				first = n + 1
			}
		}
	}
	start := time.Now()
	limit := time.Duration(*hours * float64(time.Hour))
	nb := len(strings.Split(*backends, ","))
	var log []string
	ran := map[string]int{}
	seed := first
	for {
		done := true
		for _, h := range hs {
			if ran[h.test] < *cases {
				done = false
			}
		}
		if done || time.Since(start) >= limit {
			break
		}
		for _, h := range hs {
			if ran[h.test] >= *cases {
				continue
			}
			t0 := time.Now()
			cmd := exec.Command("go", "test", "./internal/stdtest", "-run", "^"+h.test+"$", "-count=1", "-timeout", "2h")
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("%s_SEED=%d", h.env, seed),
				fmt.Sprintf("%s_N=%d", h.env, *batch),
				fmt.Sprintf("%s_BACKENDS=%s", h.env, *backends))
			b, err := cmd.CombinedOutput()
			if err != nil {
				tail := string(b)
				if len(tail) > 20000 {
					tail = tail[len(tail)-20000:]
				}
				return fmt.Errorf("%s seed %d (%d cases on %s) failed after %d cases: %v\n%s", h.test, seed, *batch, *backends, ran[h.test], err, tail)
			}
			ran[h.test] += *batch
			line := fmt.Sprintf("- %s seed %d: %d cases on %s, %s", h.test, seed, *batch, *backends, time.Since(t0).Round(time.Second))
			log = append(log, line)
			fmt.Println(line)
		}
		seed++
	}
	el := time.Since(start)
	var meas []string
	for _, h := range hs {
		meas = append(meas, fmt.Sprintf("%s %d cases per backend", h.test, ran[h.test]))
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: fuzz\nmodule: %s\nresult: pass\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven fuzz -cases %d -hours %g -batch %d -backends %s -host %q %s\n", *cases, *hours, *batch, *backends, *hostLabel, module)
	fmt.Fprintf(&b, "measure: %s, %d backends (%s), %s, 0 mismatches\n", strings.Join(meas, "; "), nb, *backends, el.Round(time.Second))
	fmt.Fprintf(&b, "seeds: %d to %d\n\n", first, seed-1)
	fmt.Fprintf(&b, "Long fuzz evidence (decision 0616): every harness that names std/%s, against Go's package, with fresh seeds of %d cases each.\n\n", module, *batch)
	b.WriteString(strings.Join(log, "\n") + "\n")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, []byte(b.String()), 0o644)
}

func gitOut(args ...string) (string, error) {
	b, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(b)), err
}

// timing runs a dudect program of decision 0616's checklist item 3.
func timing(args []string) error {
	fs := flag.NewFlagSet("timing", flag.ExitOnError)
	n := fs.Int("n", 1000000, "measurements per pair")
	backends := fs.String("backends", "arm64,llvm", "backends, comma separated")
	pairs := fs.String("pairs", "", "class pairs A:B, comma separated")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	fs.Parse(args)
	if fs.NArg() != 2 || *pairs == "" {
		usage()
	}
	module, name := fs.Arg(0), fs.Arg(1)
	dir := filepath.Join("std", "evidence", module, "timing", name)
	if _, err := os.Stat(filepath.Join(dir, "main.xo")); err != nil {
		return err
	}
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", "compiler"); dirty != "" {
		return errors.New("std or compiler has uncommitted changes: evidence names a commit")
	}
	tmp, err := os.MkdirTemp("", "stdproven-timing-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	start := time.Now()
	var lines []string
	worst := 0.0
	for _, b := range strings.Split(*backends, ",") {
		bin := filepath.Join(tmp, "t-"+b)
		if out, err := exec.Command(filepath.Join("bin", "xo"), "build", "--backend="+b, "-o", bin, dir).CombinedOutput(); err != nil {
			return fmt.Errorf("xo build --backend=%s %s: %v\n%s", b, dir, err, out)
		}
		for _, p := range strings.Split(*pairs, ",") {
			a, c, ok := strings.Cut(p, ":")
			if !ok {
				usage()
			}
			out, err := exec.Command(bin, a, c, strconv.Itoa(*n)).CombinedOutput()
			text := strings.TrimSpace(string(out))
			last := text[strings.LastIndex(text, "\n")+1:]
			fmt.Printf("%s: %s\n", b, last)
			if err != nil || !strings.HasSuffix(last, ", pass") {
				return fmt.Errorf("%s %s: %v\n%s", b, p, err, text)
			}
			if i := strings.Index(last, "max |t| "); i >= 0 {
				f := strings.Fields(last[i+len("max |t| "):])
				if len(f) > 0 {
					if t, err := strconv.ParseFloat(strings.TrimSuffix(f[0], ","), 64); err == nil && t > worst {
						worst = t
					}
				}
			}
			lines = append(lines, "- "+b+": "+last)
		}
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: timing\nmodule: %s\nresult: pass\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven timing -n %d -backends %s -pairs %s -host %q %s %s\n", *n, *backends, *pairs, host, module, name)
	fmt.Fprintf(&b, "measure: %d pairs on %s, %d measurements each, max |t| %.3f (pass below 4.5), %s\n\n", len(lines), *backends, *n, worst, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Timing evidence (decision 0616 checklist item 3): the dudect program %s, Welch's t over the times of two input classes, uncropped and cropped at several percentiles.\n\n", dir)
	b.WriteString(strings.Join(lines, "\n") + "\n")
	return os.WriteFile(filepath.Join("std", "evidence", module, "timing-"+name+".md"), []byte(b.String()), 0o644)
}

// race runs decision 0616's race kind for one module.
func race(args []string) error {
	fs := flag.NewFlagSet("race", flag.ExitOnError)
	// llvm: no ThreadSanitizer runtime ships for the native backend on
	// linux/arm64 (xo: "--race: no ThreadSanitizer runtime for linux/arm64")
	backend := fs.String("backend", "llvm", "the backend to run --race on")
	stressMin := fs.Int("stress-minutes", 10, "minutes the stress program runs under --race (0 skips it)")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", "compiler", "internal/stdtest"); dirty != "" {
		return errors.New("std, compiler or internal/stdtest has uncommitted changes: evidence names a commit")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	xo := filepath.Join(root, "bin", "xo")
	type run struct{ name, target string }
	var runs []run
	if _, err := os.Stat(filepath.Join("std", "native", module+"_test.xo")); err == nil {
		runs = append(runs, run{"module tests (std/native/" + module + "_test.xo)", "--std-module=native/" + module})
	}
	dirs, _ := filepath.Glob(filepath.Join("internal", "stdtest", "testdata", "corpus", module+"*"))
	for _, d := range dirs {
		base := filepath.Base(d)
		if _, err := os.Stat(filepath.Join("std", base+".xo")); err == nil && base != module {
			continue // another module's corpus (jsonrpc for json)
		}
		runs = append(runs, run{"corpus " + base, filepath.Join(root, d)})
	}
	if len(runs) == 0 {
		return fmt.Errorf("%s has no module tests and no corpus", module)
	}
	start := time.Now()
	var lines []string
	for _, r := range runs {
		t0 := time.Now()
		cmd := exec.Command(xo, "test", "--race", "--backend="+*backend, r.target)
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		out, err := cmd.CombinedOutput()
		text := string(out)
		if err != nil || strings.Contains(text, "DATA RACE") {
			if len(text) > 20000 {
				text = text[len(text)-20000:]
			}
			return fmt.Errorf("%s: xo test --race: %v\n%s", r.name, err, text)
		}
		sum := strings.TrimSpace(text)
		sum = sum[strings.LastIndex(sum, "\n")+1:]
		line := fmt.Sprintf("- %s: %s, %s", r.name, sum, time.Since(t0).Round(time.Second))
		fmt.Println(line)
		lines = append(lines, line)
	}
	// decision 0616: the stress program of the stress kind, also under
	// --race, for -stress-minutes
	if dir := filepath.Join("std", "evidence", module, "stress"); *stressMin > 0 {
		if _, err := os.Stat(filepath.Join(dir, "main.xo")); err == nil {
			tmp, err := os.MkdirTemp("", "stdproven-race-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			bin := filepath.Join(tmp, "s-race")
			bc := exec.Command(xo, "build", "--race", "--backend="+*backend, "-o", bin, dir)
			bc.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
			if out, err := bc.CombinedOutput(); err != nil {
				return fmt.Errorf("xo build --race --backend=%s %s: %v\n%s", *backend, dir, err, out)
			}
			t0 := time.Now()
			out, err := exec.Command(bin, strconv.Itoa(*stressMin), "64").CombinedOutput()
			text := strings.TrimSpace(string(out))
			last := text[strings.LastIndex(text, "\n")+1:]
			if err != nil || strings.Contains(text, "DATA RACE") || !strings.HasSuffix(last, ", pass") {
				if len(text) > 20000 {
					text = text[len(text)-20000:]
				}
				return fmt.Errorf("stress under --race: %v\n%s", err, text)
			}
			line := fmt.Sprintf("- stress program under --race, 64 tasks: %s, %s", last, time.Since(t0).Round(time.Second))
			fmt.Println(line)
			lines = append(lines, line)
			runs = append(runs, run{"stress", dir})
		}
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: race\nmodule: %s\nresult: pass\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven race -backend %s -stress-minutes %d -host %q %s\n", *backend, *stressMin, host, module)
	fmt.Fprintf(&b, "measure: %d runs under --race on %s, 0 data races, 0 failures, %s\n\n", len(runs), *backend, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Race evidence (decision 0616): std/%s's tests, corpora and stress program under --race (ThreadSanitizer).\n\n", module)
	b.WriteString(strings.Join(lines, "\n") + "\n")
	out := filepath.Join("std", "evidence", module, "race.md")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, []byte(b.String()), 0o644)
}

// stress runs decision 0616's stress kind: the module's stress program
// std/evidence/MODULE/stress/main.xo, built on each backend and run for
// -minutes with -tasks tasks. The program checks its own results and
// memory and prints one summary line ending in ", pass".
func stress(args []string) error {
	fs := flag.NewFlagSet("stress", flag.ExitOnError)
	minutes := fs.Int("minutes", 30, "run time per backend")
	tasks := fs.Int("tasks", 64, "concurrent tasks")
	backends := fs.String("backends", "arm64,llvm", "backends, comma separated")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	dir := filepath.Join("std", "evidence", module, "stress")
	if _, err := os.Stat(filepath.Join(dir, "main.xo")); err != nil {
		return err
	}
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", "compiler"); dirty != "" {
		return errors.New("std or compiler has uncommitted changes: evidence names a commit")
	}
	tmp, err := os.MkdirTemp("", "stdproven-stress-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	start := time.Now()
	var lines []string
	for _, b := range strings.Split(*backends, ",") {
		bin := filepath.Join(tmp, "s-"+b)
		if out, err := exec.Command(filepath.Join("bin", "xo"), "build", "--backend="+b, "-o", bin, dir).CombinedOutput(); err != nil {
			return fmt.Errorf("xo build --backend=%s %s: %v\n%s", b, dir, err, out)
		}
		t0 := time.Now()
		out, err := exec.Command(bin, strconv.Itoa(*minutes), strconv.Itoa(*tasks)).CombinedOutput()
		text := strings.TrimSpace(string(out))
		last := text[strings.LastIndex(text, "\n")+1:]
		if err != nil || !strings.HasSuffix(last, ", pass") {
			if len(text) > 20000 {
				text = text[len(text)-20000:]
			}
			return fmt.Errorf("%s stress: %v\n%s", b, err, text)
		}
		line := fmt.Sprintf("- %s: %s, %s", b, last, time.Since(t0).Round(time.Second))
		fmt.Println(line)
		lines = append(lines, line)
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: stress\nmodule: %s\nresult: pass\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven stress -minutes %d -tasks %d -backends %s -host %q %s\n", *minutes, *tasks, *backends, host, module)
	fmt.Fprintf(&b, "measure: %d tasks for %d min on each of %s, 0 mismatches, 0 faults, memory under 10%% growth after 5 min, %s\n\n", *tasks, *minutes, *backends, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Stress evidence (decision 0616): the program %s, every result checked against Go's answer, the resident set sampled every 10 s.\n\n", dir)
	b.WriteString(strings.Join(lines, "\n") + "\n")
	return os.WriteFile(filepath.Join("std", "evidence", module, "stress.md"), []byte(b.String()), 0o644)
}

// perf times the module's Go twins: every bench/perf/MODULE* directory with
// a twin.txt (scripts/twincheck checks their outputs agree, then -time
// runs each case's Xo and Go programs -rounds times, alternating, at full
// size) and writes std/evidence/MODULE/perf.md, a table of the medians
// with Xo over Go, the performance axis's evidence (decision 0461:
// Proven at or under 1.05 of Go in every row). Rows under 5 ms on both
// sides are listed but marked: startup decides them.
func perf(args []string) error {
	fs := flag.NewFlagSet("perf", flag.ExitOnError)
	rounds := fs.Int("rounds", 9, "timed runs of each program")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", "compiler", "bench/perf"); dirty != "" {
		return errors.New("std, compiler or bench/perf has uncommitted changes: evidence names a commit")
	}
	cands, _ := filepath.Glob(filepath.Join("bench", "perf", module+"*"))
	var dirs []string
	for _, d := range cands {
		if _, err := os.Stat(filepath.Join(d, "twin.txt")); err == nil {
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 {
		return fmt.Errorf("no bench/perf/%s* directory has a twin.txt", module)
	}
	start := time.Now()
	cmd := exec.Command("go", append([]string{"run", "./scripts/twincheck", "-time", strconv.Itoa(*rounds)}, dirs...)...)
	cmd.Env = append(os.Environ(), "XO_CLI="+filepath.Join("bin", "xo"))
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		if len(text) > 20000 {
			text = text[len(text)-20000:]
		}
		return fmt.Errorf("twincheck: %v\n%s", err, text)
	}
	type row struct {
		name    string
		xo, gom float64
		ratio   float64
	}
	var rows []row
	for _, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if len(f) != 8 || f[0] != "time" || f[2] != "xo" || f[4] != "go" || f[6] != "ratio" {
			continue
		}
		x, e1 := strconv.ParseFloat(f[3], 64)
		g, e2 := strconv.ParseFloat(f[5], 64)
		r, e3 := strconv.ParseFloat(f[7], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		rows = append(rows, row{f[1], x, g, r})
	}
	if len(rows) == 0 {
		return fmt.Errorf("twincheck timed no case\n%s", text)
	}
	worst := 0.0
	var b strings.Builder
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var tb strings.Builder
	tb.WriteString("| workload | Go ms | Xo ms | Xo / Go |\n|---|---|---|---|\n")
	for _, r := range rows {
		mark := ""
		if r.xo < 5 && r.gom < 5 {
			mark = " (under 5 ms: startup)"
		} else if r.ratio > worst {
			worst = r.ratio
		}
		fmt.Fprintf(&tb, "| %s%s | %.1f | %.1f | %.3f |\n", r.name, mark, r.gom, r.xo, r.ratio)
	}
	result := "pass"
	if worst > 1.05 {
		result = "fail"
	}
	fmt.Fprintf(&b, "kind: perf\nmodule: %s\nresult: %s\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, result, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven perf -rounds %d -host %q %s\n", *rounds, host, module)
	fmt.Fprintf(&b, "measure: %d cases of %d twin directories, worst Xo / Go %.3f over the cases above 5 ms (Proven at or under 1.05), %s\n\n", len(rows), len(dirs), worst, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Performance against Go (decisions 0461, 0616): scripts/twincheck -time %d over %s; medians of wall time, the two programs alternating.\n\n", *rounds, strings.Join(dirs, ", "))
	b.WriteString(tb.String())
	outp := filepath.Join("std", "evidence", module, "perf.md")
	if err := os.MkdirAll(filepath.Dir(outp), 0o755); err != nil {
		return err
	}
	fmt.Print(tb.String())
	return os.WriteFile(outp, []byte(b.String()), 0o644)
}
