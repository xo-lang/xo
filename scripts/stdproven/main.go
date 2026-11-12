// Command stdproven runs the testing evidence of decision 0616 for one std
// module and writes it to std/evidence/MODULE/KIND.md:
//
//	go run ./scripts/stdproven hash MODULE
//	go run ./scripts/stdproven fuzz [-cases N] [-hours H] [-batch N] [-backends B,...] MODULE
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
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdproven:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: stdproven hash MODULE | stdproven fuzz [-cases N] [-hours H] [-batch N] [-backends B,...] MODULE")
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
