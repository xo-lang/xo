package gen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// buildText is why a build failed: its diagnostics, else xo's own output
// (a compiler fault is on standard error, not a diagnostic: the property
// tests printed "the arm64 build failed: <nil>" for one, 2026-10-18).
func buildText(res *xotool.Result) string {
	if d := diagText(res); d != "" || res == nil {
		return d
	}
	return res.Output
}

// triage keeps a property test's failing program and diagnoses it, for
// the failure message:
//
//   - the program is written to $XO_PROP_SAVE (default
//     $TMPDIR/xo-prop-failures)/TEST-SEED/, whatever else happens;
//   - on the arm64 and amd64 backends, `xo dev bisect-pass` names the
//     first optional pass whose output diverges from the Go backend's;
//   - scripts/reduce-diff.py shrinks a single file program while it
//     still fails the same way (--crash for a build that fails), within
//     $XO_PROP_TRIAGE_MIN minutes (default 10).
//
// XO_PROP_TRIAGE=off keeps only the program.
func triage(test string, seed int64, files map[string]string, be string, crash bool) string {
	var b strings.Builder
	base := os.Getenv("XO_PROP_SAVE")
	if base == "" {
		base = filepath.Join(os.TempDir(), "xo-prop-failures")
	}
	dir := filepath.Join(base, fmt.Sprintf("%s-%d", test, seed))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf("--- triage\n(could not keep the program: %v)\n", err)
	}
	for name, text := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
	}
	fmt.Fprintf(&b, "--- triage\nprogram kept in %s\n", dir)
	if os.Getenv("XO_PROP_TRIAGE") == "off" {
		return b.String()
	}
	xo, err := xotool.Path()
	if err != nil {
		fmt.Fprintf(&b, "(no xo: %v)\n", err)
		return b.String()
	}
	mins := 10
	if n, err := strconv.Atoi(os.Getenv("XO_PROP_TRIAGE_MIN")); err == nil && n > 0 {
		mins = n
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(mins)*time.Minute)
	defer cancel()
	if be == xotool.BackendArm64 || be == "amd64" {
		cmd := testtmp.Guard(exec.CommandContext(ctx, xo, "dev", "bisect-pass", "--backend="+be, dir))
		out, _ := cmd.CombinedOutput()
		b.Write(out)
	}
	main, single := files["main.xo"]
	if !single || len(files) != 2 { // main.xo and xo.mod
		fmt.Fprintf(&b, "(not reduced: the program is more than main.xo)\n")
		return b.String()
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		fmt.Fprintf(&b, "(not reduced: no python3)\n")
		return b.String()
	}
	in := filepath.Join(dir, "main.xo")
	min := filepath.Join(dir, "main.min.xo")
	args := []string{"-I", filepath.Join("..", "..", "scripts", "reduce-diff.py"), "-q"}
	if crash {
		args = append(args, "--crash")
	}
	args = append(args, xo, "go", be, in, min)
	cmd := exec.CommandContext(ctx, py, args...)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		fmt.Fprintf(&b, "(the reduction ran out of its %d minutes; the smallest so far is in %s)\n", mins, min)
		return b.String()
	}
	if err != nil {
		fmt.Fprintf(&b, "(not reduced: %v)\n", err)
		return b.String()
	}
	fmt.Fprintf(&b, "reduced from %d to %d lines (%s):\n%s", strings.Count(main, "\n"), strings.Count(string(out), "\n"), min, out)
	return b.String()
}
