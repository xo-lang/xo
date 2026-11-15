package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
)

// openEnumProg is a program whose match subjects are wide, deep types (an
// enum of 12 variants each holding a struct of 12 fields of the next
// level's enum, six levels) with an open enum of another package loaded
// (std/bufio's BufErr) and `matches` exhaustive matches on the top enum,
// or, when !deep, on an enum of the same width whose variants hold an Int.
func openEnumProg(matches int, deep bool) string {
	const w, d = 12, 6
	var b strings.Builder
	b.WriteString("use std/bufio\n\n")
	for lv := 0; lv < d; lv++ {
		fmt.Fprintf(&b, "struct S%d {\n", lv)
		for i := 0; i < w; i++ {
			if lv == d-1 {
				fmt.Fprintf(&b, "  f%d: Int\n", i)
			} else {
				fmt.Fprintf(&b, "  f%d: E%d\n", i, lv+1)
			}
		}
		fmt.Fprintf(&b, "}\nenum E%d {\n", lv)
		for i := 0; i < w; i++ {
			fmt.Fprintf(&b, "  V%d(s: S%d)\n", i, lv)
		}
		b.WriteString("}\n")
	}
	b.WriteString("enum Flat {\n")
	for i := 0; i < w; i++ {
		fmt.Fprintf(&b, "  W%d(n: Int)\n", i)
	}
	b.WriteString("}\n")
	subj, ctor := "Flat", "W"
	if deep {
		subj, ctor = "E0", "V"
	}
	for k := 0; k < matches; k++ {
		fmt.Fprintf(&b, "\nfn m%d(x: %s, e: bufio.BufErr?) -> Int {\n  if let Some(_) = e {\n    return -1\n  }\n  match x {\n", k, subj)
		for i := 0; i < w; i++ {
			fmt.Fprintf(&b, "    %s%d(_) => %d\n", ctor, i, i)
		}
		b.WriteString("  }\n}\n")
	}
	b.WriteString("\nfn main(os: Os) {\n  os.stdio.println(\"ok\")\n}\n")
	return b.String()
}

var retiredRE = regexp.MustCompile(`(\d+)\s+instructions retired`)

// checkCost runs `xo check` on dir and returns its cost: the instructions
// retired on darwin (/usr/bin/time -l), else the user CPU seconds.
func checkCost(t *testing.T, dir string) float64 {
	t.Helper()
	xo := xoBin(t)
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("/usr/bin/time", "-l", xo, "check", dir)
	} else {
		cmd = exec.Command(xo, "check", dir)
	}
	cmd = testtmp.Guard(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("xo check %s: %v\n%s%s", dir, err, stdout.String(), stderr.String())
	}
	if runtime.GOOS == "darwin" {
		m := retiredRE.FindStringSubmatch(stderr.String())
		if m == nil {
			t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		return float64(n)
	}
	return cmd.ProcessState.UserTime().Seconds()
}

// TestOpenEnumCheckCost guards dfc2c648e (decision 0649's XO0306 check):
// open_enum_in walked a match subject's type, its variants' and fields'
// types to depth 4, at every match with no memo, copying each object out
// of c.objs, so `xo check compiler/xo` took 10 s against 2.4 s before
// 3ad672e5e. The cost is a ratio, 1000 matches on the deep enum against
// 1000 matches on a flat enum of the same width in the same program, so
// host speed, the match checking itself and the std check (cached or not)
// cancel. Darwin instruction counts, 2026-11-15: 37.9 at 00df73417
// (44.0G against 1.16G), 1.77 with the fix (2.12G): the ceiling is 5.
// User CPU time (other systems) counts the std check's fixed cost in both,
// so its ratio is smaller: 1.01 with the fix on linux/arm64, about 4 at
// 00df73417 by the Mac's wall times (3.9 s against 0.9 s): the ceiling is
// 2.5.
func TestOpenEnumCheckCost(t *testing.T) {
	if testing.Short() {
		t.Skip("runs xo check on two generated programs")
	}
	root := t.TempDir()
	flat := filepath.Join(root, "flat")
	deep := filepath.Join(root, "deep")
	for dir, d := range map[string]bool{flat: false, deep: true} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module oecost\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(openEnumProg(1000, d)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	checkCost(t, flat) // warms the std check
	base := checkCost(t, flat)
	cost := checkCost(t, deep)
	ratio := cost / base
	ceiling := 2.5
	if runtime.GOOS == "darwin" {
		ceiling = 5
	}
	t.Logf("xo check: %.4g for 1000 matches on the deep enum, %.4g on the flat one: ratio %.2f", cost, base, ratio)
	if ratio > ceiling {
		t.Errorf("xo check of 1000 matches on a deep enum costs %.2f times the same on a flat enum (ceiling %g): the XO0306 walk of each match subject's type (open_enum_in) is not memoized", ratio, ceiling)
	}
}
