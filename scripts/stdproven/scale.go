package main

// The scale kind (task #351, class guard 3 of the 2026-11-18 class guard
// survey): every quadratic or count-sized allocation fixed in std was
// found by a reviewer, and the fuzz kind only flags a case past 10 s. A
// module's scale program, std/evidence/MODULE/scale/main.xo, names its
// cases and runs one at a time:
//
//	main list                 one line a case: NAME n=N mem=X [stream]
//	main NAME N one|bytes     runs the case on an input of size N (one
//	                          write, or 1-byte writes for a stream case)
//	                          and prints input=BYTES work_ns=T, T the
//	                          time of the work alone (not the input's
//	                          generation)
//
// For each case the runner runs N and 8N, each the best of 3, on the
// native backend, and fails when the work at 8N takes over scaleTimeRatio
// times the work at N (n log n headroom), when a stream case's 1-byte
// writes take over scaleStreamRatio times its one write, or when the peak
// resident set grows by more than X times the input's growth plus 4 MiB.

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	scaleTimeRatio   = 12.0
	scaleStreamRatio = 4.0
	scaleRuns        = 3
	// scaleMemSlack: peak growth up to this much is allocator and page
	// noise, not a count-sized allocation
	scaleMemSlack = 4 << 20
)

// scaleCase is one line of a scale program's list.
type scaleCase struct {
	name   string
	n      int
	mem    float64
	stream bool
}

// scaleRun is one run of a case: the input's size, the work's time and
// the process's peak resident set in bytes.
type scaleRun struct {
	input, workNs, peak int64
}

// parseScaleList reads a scale program's list output.
func parseScaleList(text string) ([]scaleCase, error) {
	var cs []scaleCase
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		c := scaleCase{name: f[0]}
		for _, w := range f[1:] {
			k, v, _ := strings.Cut(w, "=")
			switch k {
			case "n":
				n, err := strconv.Atoi(v)
				if err != nil || n <= 0 {
					return nil, fmt.Errorf("case %s: n=%q is not a positive count", c.name, v)
				}
				c.n = n
			case "mem":
				m, err := strconv.ParseFloat(v, 64)
				if err != nil || m <= 0 {
					return nil, fmt.Errorf("case %s: mem=%q is not a positive multiple", c.name, v)
				}
				c.mem = m
			case "stream":
				c.stream = true
			default:
				return nil, fmt.Errorf("case %s: unknown field %q (want n=N mem=X [stream])", c.name, w)
			}
		}
		if c.n == 0 || c.mem == 0 {
			return nil, fmt.Errorf("case %s: needs n=N and mem=X", c.name)
		}
		cs = append(cs, c)
	}
	if len(cs) == 0 {
		return nil, errors.New("the list names no case")
	}
	return cs, nil
}

// parseScaleRun reads input=BYTES work_ns=T from a case run's output.
func parseScaleRun(text string) (scaleRun, error) {
	var r scaleRun
	seen := 0
	for _, w := range strings.Fields(text) {
		k, v, ok := strings.Cut(w, "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		switch k {
		case "input":
			r.input, seen = n, seen|1
		case "work_ns":
			r.workNs, seen = n, seen|2
		}
	}
	if seen != 3 {
		return r, fmt.Errorf("no input=BYTES work_ns=T in %q", strings.TrimSpace(text))
	}
	return r, nil
}

// scaleJudge applies the kind's bars to a case's runs at N and 8N (and a
// stream case's 1-byte run at N); it returns the evidence line and the
// failures.
func scaleJudge(c scaleCase, at1, at8 scaleRun, bytes *scaleRun) (string, []string) {
	var fails []string
	tr := float64(at8.workNs) / float64(max(at1.workNs, 1))
	if tr > scaleTimeRatio {
		fails = append(fails, fmt.Sprintf("time grew %.1fx from n to 8n (over %.0fx)", tr, scaleTimeRatio))
	}
	dIn := at8.input - at1.input
	dMem := at8.peak - at1.peak
	mr := 0.0
	if dIn > 0 {
		mr = float64(dMem) / float64(dIn)
	}
	if float64(dMem) > c.mem*float64(dIn)+scaleMemSlack {
		fails = append(fails, fmt.Sprintf("peak memory grew %.1fx the input's growth (over the declared %gx)", mr, c.mem))
	}
	line := fmt.Sprintf("- %s: n=%d %s, 8n %s: time %.1fx, memory %.1fx the input", c.name, c.n, time.Duration(at1.workNs).Round(time.Microsecond), time.Duration(at8.workNs).Round(time.Microsecond), tr, mr)
	if bytes != nil {
		sr := float64(bytes.workNs) / float64(max(at1.workNs, 1))
		if sr > scaleStreamRatio {
			fails = append(fails, fmt.Sprintf("1-byte writes took %.1fx the one write (over %.0fx)", sr, scaleStreamRatio))
		}
		line += fmt.Sprintf(", 1-byte writes %.1fx the one write", sr)
	}
	if len(fails) > 0 {
		line += ", FAIL (" + strings.Join(fails, "; ") + ")"
	} else {
		line += ", pass"
	}
	return line, fails
}

// peakRSS is a finished process's peak resident set in bytes (getrusage's
// ru_maxrss: KiB on Linux, bytes on darwin).
func peakRSS(ps *os.ProcessState) int64 {
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return int64(ru.Maxrss)
	}
	return int64(ru.Maxrss) * 1024
}

// scaleOnce runs bin on one case, the best (least work time, least peak)
// of scaleRuns runs.
func scaleOnce(bin string, c scaleCase, n int, mode string) (scaleRun, error) {
	var best scaleRun
	for i := 0; i < scaleRuns; i++ {
		cmd := exec.Command(bin, c.name, strconv.Itoa(n), mode)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return best, fmt.Errorf("%s %s %d %s: %v\n%s", filepath.Base(bin), c.name, n, mode, err, out)
		}
		r, err := parseScaleRun(string(out))
		if err != nil {
			return best, fmt.Errorf("%s %d %s: %v", c.name, n, mode, err)
		}
		r.peak = peakRSS(cmd.ProcessState)
		if i == 0 || r.workNs < best.workNs {
			best.workNs = r.workNs
		}
		if i == 0 || r.peak < best.peak {
			best.peak = r.peak
		}
		best.input = r.input
	}
	return best, nil
}

// scaleCheck builds the scale program in dir and judges every case; it
// returns the evidence lines and whether every case passed.
func scaleCheck(xo, dir, tmp string) ([]string, bool, error) {
	bin := filepath.Join(tmp, "scale")
	nb := nativeBackend(runtime.GOARCH)
	if out, err := exec.Command(xo, "build", "--backend="+nb, "-o", bin, dir).CombinedOutput(); err != nil {
		return nil, false, fmt.Errorf("xo build --backend=%s %s: %v\n%s", nb, dir, err, out)
	}
	out, err := exec.Command(bin, "list").CombinedOutput()
	if err != nil {
		return nil, false, fmt.Errorf("%s list: %v\n%s", dir, err, out)
	}
	cs, err := parseScaleList(string(out))
	if err != nil {
		return nil, false, fmt.Errorf("%s list: %v", dir, err)
	}
	var lines []string
	ok := true
	for _, c := range cs {
		at1, err := scaleOnce(bin, c, c.n, "one")
		if err != nil {
			return nil, false, err
		}
		at8, err := scaleOnce(bin, c, 8*c.n, "one")
		if err != nil {
			return nil, false, err
		}
		var by *scaleRun
		if c.stream {
			r, err := scaleOnce(bin, c, c.n, "bytes")
			if err != nil {
				return nil, false, err
			}
			by = &r
		}
		line, fails := scaleJudge(c, at1, at8, by)
		fmt.Println(line)
		lines = append(lines, line)
		if len(fails) > 0 {
			ok = false
		}
	}
	return lines, ok, nil
}

// scale runs the scale kind for one module and writes its scale.md.
func scale(args []string) error {
	fs := flag.NewFlagSet("scale", flag.ExitOnError)
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	dir := filepath.Join("std", "evidence", module, "scale")
	if _, err := os.Stat(filepath.Join(dir, "main.xo")); err != nil {
		return fmt.Errorf("%s: no scale program (main.xo); the module's owner writes it (task #351)", dir)
	}
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", ":(exclude)std/evidence", "compiler"); dirty != "" {
		return fmt.Errorf("std or compiler has uncommitted changes: evidence names a commit:\n%s", dirty)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "stdproven-scale-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	start := time.Now()
	lines, ok, err := scaleCheck(filepath.Join(root, "bin", "xo"), dir, tmp)
	if err != nil {
		return err
	}
	result := "pass"
	if !ok {
		result = "fail"
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: scale\nmodule: %s\nresult: %s\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, result, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven scale -host %q %s\n", host, module)
	fmt.Fprintf(&b, "measure: %d cases at n and 8n on %s, best of %d; bars: time under %.0fx, 1-byte writes under %.0fx the one write, peak memory growth under each case's declared multiple of the input's, %s\n\n", len(lines), nativeBackend(runtime.GOARCH), scaleRuns, scaleTimeRatio, scaleStreamRatio, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Scale evidence (task #351): the program %s, each case's work timed at n and 8n.\n\n", dir)
	b.WriteString(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(filepath.Join("std", "evidence", module, "scale.md"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("std/%s: scale failed:\n%s", module, strings.Join(lines, "\n"))
	}
	return nil
}
