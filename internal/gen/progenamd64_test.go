package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/progen"
	"github.com/xo-lang/xo/internal/xotool"
)

// The amd64 backend builds only on a linux/amd64 host, so progen runs it
// in three steps (scripts/progen-amd64.sh): TestProgenAmd64Dump writes
// the programs and their Go backend builds for linux/amd64 on any host;
// a linux/amd64 container builds each with --backend=amd64 and runs both
// binaries (scripts/progen-amd64-run.sh), writing their exit status,
// stdout and stderr; TestProgenAmd64Compare compares the two runs as the
// property tests do (normStderr) and keeps a diverging program.
//
//	XO_PROGEN_AMD64=DIR XO_PROGEN_KIND=compile|arith|defer XO_PROGEN_FIRST=1 XO_PROGEN_N=250

// progenAmd64 is the configuration of both tests from the environment,
// or skips the test.
func progenAmd64(t *testing.T) (dir, kind string, first, n int) {
	dir = os.Getenv("XO_PROGEN_AMD64")
	if dir == "" {
		t.Skip("XO_PROGEN_AMD64 is not set (scripts/progen-amd64.sh)")
	}
	kind = os.Getenv("XO_PROGEN_KIND")
	if kind == "" {
		kind = "compile"
	}
	first, n = 1, 250
	if v, err := strconv.Atoi(os.Getenv("XO_PROGEN_FIRST")); err == nil && v > 0 {
		first = v
	}
	if v, err := strconv.Atoi(os.Getenv("XO_PROGEN_N")); err == nil && v > 0 {
		n = v
	}
	return dir, kind, first, n
}

// progenSource is program seed of a progen kind.
func progenSource(kind string, seed int64) (string, error) {
	switch kind {
	case "compile":
		return progen.Program(seed), nil
	case "arith":
		return progen.Arith(seed), nil
	case "defer":
		return progen.DeferFaultProgram(seed), nil
	}
	return "", fmt.Errorf("XO_PROGEN_KIND=%s: compile, arith, or defer", kind)
}

func TestProgenAmd64Dump(t *testing.T) {
	dir, kind, first, n := progenAmd64(t)
	for seed := first; seed < first+n; seed++ {
		src, err := progenSource(kind, int64(seed))
		if err != nil {
			t.Fatal(err)
		}
		d := filepath.Join(dir, fmt.Sprintf("%s-%d", kind, seed))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(d, "xo.mod"), []byte("module example.com/prop\n"), 0o644)
		os.WriteFile(filepath.Join(d, "main.xo"), []byte(src), 0o644)
		res, err := xotool.Build([]string{d}, xotool.Options{Backend: "go", Targets: "linux/amd64", Out: filepath.Join(d, "prog-go")})
		if err != nil || res.Failed {
			t.Errorf("seed %d: the Go build for linux/amd64 failed: %v\n%s", seed, err, buildText(res))
		}
	}
}

// amd64Run is one recorded run: "build failed" with xo's output, or the
// exit status, stdout and stderr the container wrote.
func amd64Run(d, name string) string {
	read := func(f string) string { b, _ := os.ReadFile(filepath.Join(d, name+"."+f)); return string(b) }
	if b := read("build"); b != "" {
		return "build failed:\n" + b
	}
	return fmt.Sprintf("exit %s\n--- stdout\n%s--- stderr\n%s", strings.TrimSpace(read("exit")), read("stdout"), normStderr(read("stderr")))
}

func TestProgenAmd64Compare(t *testing.T) {
	dir, kind, first, n := progenAmd64(t)
	var bad []int
	for seed := first; seed < first+n; seed++ {
		d := filepath.Join(dir, fmt.Sprintf("%s-%d", kind, seed))
		if _, err := os.Stat(filepath.Join(d, "go.exit")); err != nil {
			t.Errorf("seed %d: no run recorded in %s", seed, d)
			continue
		}
		g, a := amd64Run(d, "go"), amd64Run(d, "amd64")
		if g != a {
			bad = append(bad, seed)
			src, _ := os.ReadFile(filepath.Join(d, "main.xo"))
			files := map[string]string{"xo.mod": "module example.com/prop\n", "main.xo": string(src)}
			os.Setenv("XO_PROP_TRIAGE", "off") // bisect and reduce need an amd64 host
			t.Errorf("seed %d: the amd64 build runs differently\n--- go\n%s\n--- amd64\n%s\n--- program\n%s%s", seed, g, a, numbered(string(src)), triage(kind+"-amd64", int64(seed), files, "amd64", false))
		}
	}
	sort.Ints(bad)
	t.Logf("%s seeds %d..%d: %d diverge %v", kind, first, first+n-1, len(bad), bad)
}
