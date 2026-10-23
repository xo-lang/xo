// Command scale runs the scale suite (knowledgebase/infrastructure/
// scale-suite.md): for each knob of internal/progen's Scale, programs of
// doubling size are built with each backend, recording the build's wall
// time, peak memory and the binary's size, and each binary's output is
// checked against progen.ScaleWant. A growth faster than linear between
// two sizes is flagged.
//
//	go run ./scripts/scale -xo bin/xo -out DIR [-knobs fnlen,vars] [-from 1024] [-to 131072]
//	    [-sizes 65535,65536] [-backends arm64,llvm,go] [-amd64] [-budget 600]
//
// -amd64 also builds each program with --backend=amd64 for linux/amd64
// and runs every such binary in one Docker container at the end
// (scripts/docker-linux.sh). A knob's sweep stops at the first size
// whose build exceeds -budget seconds on any backend (the step that
// shows the blowup is kept). Results: DIR/results.tsv, one row per
// build, and the flags on stdout and in DIR/flags.txt. Exit status 1
// when an output differs or a build fails, 3 when only growth is
// flagged.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/xo-lang/xo/internal/progen"
)

// The growth thresholds: between two sizes n1 < n2, a measure grows as
// (n2/n1)^e; e above the limit is flagged when the larger measure is
// big enough to be more than noise.
const (
	timeExp   = 1.4  // build seconds
	timeFloor = 2.0  // seconds
	rssExp    = 1.3  // peak memory
	rssFloor  = 512  // MB
	sizeExp   = 1.15 // binary size
	sizeFloor = 4    // MB
)

type row struct {
	knob    string
	n       int
	backend string
	secs    float64
	rssMB   float64
	size    int64
	status  string // ok, differs, build-failed, timeout, run-failed
}

func main() {
	xo := flag.String("xo", "", "the xo to test (required)")
	out := flag.String("out", "", "work and result directory (required)")
	knobs := flag.String("knobs", strings.Join(progen.ScaleKnobs, ","), "knobs to sweep")
	from := flag.Int("from", 1024, "first size")
	to := flag.Int("to", 131072, "last size (doubling from -from)")
	sizes := flag.String("sizes", "", "extra sizes, comma separated (count field edges)")
	backends := flag.String("backends", "arm64", "host backends: arm64, llvm, go")
	amd64 := flag.Bool("amd64", false, "also build for linux/amd64 and run in Docker")
	budget := flag.Float64("budget", 600, "seconds per build before a knob's sweep stops")
	flag.Parse()
	if *xo == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	xoPath, err := filepath.Abs(*xo)
	check(err)
	check(os.MkdirAll(*out, 0o755))
	outDir, err := filepath.Abs(*out)
	check(err)

	ns := []int{}
	for n := *from; n <= *to; n *= 2 {
		ns = append(ns, n)
	}
	for _, s := range strings.Split(*sizes, ",") {
		if s != "" {
			n, err := strconv.Atoi(s)
			check(err)
			ns = append(ns, n)
		}
	}
	sort.Ints(ns)
	ns = slices.Compact(ns)
	bes := strings.Split(*backends, ",")
	if *amd64 {
		bes = append(bes, "amd64")
	}

	tsv, err := os.Create(filepath.Join(outDir, "results.tsv"))
	check(err)
	defer tsv.Close()
	fmt.Fprintln(tsv, "knob\tn\tbackend\tbuild_s\tpeak_mb\tsize\tstatus")
	var rows []row
	var docker []string // amd64 binaries to run: dir names under outDir
	for _, knob := range strings.Split(*knobs, ",") {
		for _, n := range ns {
			dir := filepath.Join(outDir, fmt.Sprintf("%s-%d", knob, n))
			check(os.MkdirAll(dir, 0o755))
			check(os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module scale\n"), 0o644))
			check(os.WriteFile(filepath.Join(dir, "main.xo"), []byte(progen.Scale(knob, n)), 0o644))
			check(os.WriteFile(filepath.Join(dir, "want.txt"), []byte(progen.ScaleWant(knob, n)), 0o644))
			over := false
			for _, be := range bes {
				r := build(xoPath, dir, knob, n, be, *budget)
				if r.status == "ok" && be != "amd64" {
					r.status = run(filepath.Join(dir, "prog-"+be), progen.ScaleWant(knob, n))
				}
				if r.status == "ok" && be == "amd64" {
					docker = append(docker, filepath.Base(dir))
					r.status = "built"
				}
				rows = append(rows, r)
				fmt.Fprintf(tsv, "%s\t%d\t%s\t%.2f\t%.0f\t%d\t%s\n", r.knob, r.n, r.backend, r.secs, r.rssMB, r.size, r.status)
				fmt.Printf("%s %d %s: %.2f s, %.0f MB, %d bytes, %s\n", r.knob, r.n, r.backend, r.secs, r.rssMB, r.size, r.status)
				if r.secs > *budget || r.status == "timeout" {
					over = true
				}
			}
			if over {
				fmt.Printf("%s: stopped at %d (a build over %.0f s)\n", knob, n, *budget)
				break
			}
		}
	}
	if len(docker) > 0 {
		dockerRun(outDir, docker, rows)
	}

	var flags []string
	failed := false
	for _, r := range rows {
		if r.status != "ok" {
			failed = true
			flags = append(flags, fmt.Sprintf("FAIL %s %d %s: %s", r.knob, r.n, r.backend, r.status))
		}
	}
	flags = append(flags, growth(rows)...)
	check(os.WriteFile(filepath.Join(outDir, "flags.txt"), []byte(strings.Join(flags, "\n")+"\n"), 0o644))
	for _, f := range flags {
		fmt.Println(f)
	}
	switch {
	case failed:
		os.Exit(1)
	case len(flags) > 0:
		os.Exit(3)
	}
}

// build builds dir's program with backend be into dir/prog-<be>.
func build(xo, dir, knob string, n int, be string, budget float64) row {
	r := row{knob: knob, n: n, backend: be}
	args := []string{"build", "--backend=" + be, "-o", "prog-" + be, "."}
	if be == "amd64" {
		args = []string{"build", "--backend=amd64", "--targets=linux/amd64", "-o", "prog-" + be, "."}
	}
	// a build may run past the budget by half, so the step that shows a
	// blowup is still measured
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(budget*1.5*float64(time.Second)))
	defer cancel()
	cmd := exec.CommandContext(ctx, xo, args...)
	cmd.Dir = dir
	// the caches off, so a rerun of a sweep measures the same work (a
	// second build of one program took 60 s and 1.4 GB where the first
	// took 100 s and 8.4 GB, 2026-10-23); the runtime objects every
	// program shares stay cached, a constant cost
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=off", "XO_BIN_CACHE=off", "XO_FE_CACHE=off")
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	t0 := time.Now()
	err := cmd.Run()
	r.secs = time.Since(t0).Seconds()
	if cmd.ProcessState != nil {
		if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			// ru_maxrss is bytes on darwin and KB on linux; children
			// (the assembler, the linker, clang) are not counted
			b := float64(ru.Maxrss)
			if runtime.GOOS != "darwin" {
				b *= 1024
			}
			r.rssMB = b / (1 << 20)
		}
	}
	switch {
	case ctx.Err() != nil:
		r.status = "timeout"
	case err != nil:
		r.status = "build-failed"
		check(os.WriteFile(filepath.Join(dir, "build-"+be+".log"), stderr.Bytes(), 0o644))
	default:
		r.status = "ok"
		if st, err := os.Stat(filepath.Join(dir, "prog-"+be)); err == nil {
			r.size = st.Size()
		}
	}
	return r
}

// run runs a host binary and compares its output with want.
func run(bin, want string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	got, err := exec.CommandContext(ctx, bin).Output()
	if err != nil {
		return "run-failed"
	}
	if string(got) != want {
		return "differs"
	}
	return "ok"
}

// dockerRun runs every amd64 binary in one linux/amd64 container and
// sets the rows' statuses from the outputs.
func dockerRun(outDir string, dirs []string, rows []row) {
	var sh strings.Builder
	for _, d := range dirs {
		fmt.Fprintf(&sh, "if ./%s/prog-amd64 > %s/got-amd64.txt 2>&1 && cmp -s %s/got-amd64.txt %s/want.txt; then echo '%s ok'; else echo '%s differs'; fi; ", d, d, d, d, d, d)
	}
	repo, err := os.Getwd()
	check(err)
	cmd := exec.Command(filepath.Join(repo, "scripts/docker-linux.sh"), "amd64", outDir, "--", sh.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("docker: %v\n%s", err, out)
	}
	res := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) == 2 && (f[1] == "ok" || f[1] == "differs") {
			res[f[0]] = f[1]
		}
	}
	for i := range rows {
		if rows[i].backend == "amd64" && rows[i].status == "built" {
			if s, ok := res[fmt.Sprintf("%s-%d", rows[i].knob, rows[i].n)]; ok {
				rows[i].status = s
			} else {
				rows[i].status = "run-failed"
			}
		}
	}
}

// growth flags measures that grow faster than the thresholds between
// consecutive sizes of a knob and backend.
func growth(rows []row) []string {
	var flags []string
	last := map[string]row{}
	for _, r := range rows {
		if r.status != "ok" {
			continue
		}
		key := r.knob + " " + r.backend
		p, ok := last[key]
		if ok && float64(r.n) < 1.5*float64(p.n) {
			// an edge size next to another (65535, 65536): too close for
			// an exponent; it is checked for its output only
			continue
		}
		if ok {
			k := math.Log(float64(r.n) / float64(p.n))
			exp := func(a, b float64) float64 { return math.Log(b/a) / k }
			if r.secs >= timeFloor && p.secs > 0 && exp(p.secs, r.secs) > timeExp {
				flags = append(flags, fmt.Sprintf("GROWTH %s build time %d->%d: %.2f->%.2f s (n^%.2f)", key, p.n, r.n, p.secs, r.secs, exp(p.secs, r.secs)))
			}
			if r.rssMB >= rssFloor && p.rssMB > 0 && exp(p.rssMB, r.rssMB) > rssExp {
				flags = append(flags, fmt.Sprintf("GROWTH %s peak memory %d->%d: %.0f->%.0f MB (n^%.2f)", key, p.n, r.n, p.rssMB, r.rssMB, exp(p.rssMB, r.rssMB)))
			}
			if float64(r.size) >= sizeFloor*(1<<20) && p.size > 0 && exp(float64(p.size), float64(r.size)) > sizeExp {
				flags = append(flags, fmt.Sprintf("GROWTH %s binary size %d->%d: %d->%d (n^%.2f)", key, p.n, r.n, p.size, r.size, exp(float64(p.size), float64(r.size))))
			}
		}
		last[key] = r
	}
	return flags
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "scale:", err)
		os.Exit(2)
	}
}
